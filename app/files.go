package main

import (
	"encoding/base64"
	"errors"
	"os"
	"path"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/asutarisucu/DRAG/app/internal/archive"
	"github.com/asutarisucu/DRAG/app/internal/remote"
)

// デスクトップで開くファイルのウィンドウ (エディター、ログ、画像、圧縮ファイル) の窓口

const (
	textLimit  = 4 << 20   // エディターで開く大きさの上限
	imageLimit = 20 << 20  // 画像
	archLimit  = 512 << 20 // 圧縮ファイルを PC に落とす上限
)

// fileState は、開いているログの追従と、PC に落とした圧縮ファイル
type fileState struct {
	mu       sync.Mutex
	follows  map[string]func()
	archives map[string]archiveFile
}

type archiveFile struct {
	local string
	name  string
}

func (a *App) files() *fileState {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fs == nil {
		a.fs = &fileState{follows: map[string]func(){}, archives: map[string]archiveFile{}}
	}
	return a.fs
}

func (a *App) OpenText(serverID, p string) (remote.TextFile, error) {
	s, err := a.server(serverID)
	if err != nil {
		return remote.TextFile{}, err
	}
	return s.ReadText(p, textLimit)
}

// SaveText は保存して新しい更新時刻を返す。開いたあとに変わっていたら、force を付けて呼び直すまで書かない
func (a *App) SaveText(serverID, p, text string, modTime int64, force bool) (int64, error) {
	s, err := a.server(serverID)
	if err != nil {
		return 0, err
	}
	return s.WriteText(p, text, modTime, force)
}

// ReadImage は画像を base64 で返す
func (a *App) ReadImage(serverID, p string) (string, error) {
	s, err := a.server(serverID)
	if err != nil {
		return "", err
	}
	b, err := s.ReadBytes(p, imageLimit)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// LogTail は end より前の size バイトほどを行の頭から返す。end が負ならファイルの末尾から
func (a *App) LogTail(serverID, p string, end, size int64) (remote.LogChunk, error) {
	s, err := a.server(serverID)
	if err != nil {
		return remote.LogChunk{}, err
	}
	return s.ReadTail(p, end, size)
}

// LogFollow は from バイト目から後ろの追記を "log:out" (番号, 文字列) で送り始め、番号を返す。
// 止まったら "log:end" (番号, エラーの文) を送る
func (a *App) LogFollow(serverID, p string, from int64) (string, error) {
	s, err := a.server(serverID)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	id := a.newID("l")
	a.mu.Unlock()
	fs := a.files()
	stop, err := s.Follow(p, from,
		func(b []byte) { runtime.EventsEmit(a.ctx, "log:out", id, string(b)) },
		func(err error) {
			fs.mu.Lock()
			delete(fs.follows, id)
			fs.mu.Unlock()
			msg := ""
			if err != nil {
				msg = err.Error()
			}
			runtime.EventsEmit(a.ctx, "log:end", id, msg)
		})
	if err != nil {
		return "", err
	}
	fs.mu.Lock()
	fs.follows[id] = stop
	fs.mu.Unlock()
	return id, nil
}

func (a *App) LogStop(id string) {
	fs := a.files()
	fs.mu.Lock()
	stop := fs.follows[id]
	delete(fs.follows, id)
	fs.mu.Unlock()
	if stop != nil {
		stop()
	}
}

type ArchiveInfo struct {
	ID      string          `json:"id"`
	Entries []archive.Entry `json:"entries"`
}

// ArchiveOpen は圧縮ファイルを PC の一時フォルダに落とし、中身の一覧を返す。サーバーには展開しない
func (a *App) ArchiveOpen(serverID, p string) (ArchiveInfo, error) {
	if !archive.Supported(p) {
		return ArchiveInfo{}, archive.ErrUnsupported
	}
	s, err := a.server(serverID)
	if err != nil {
		return ArchiveInfo{}, err
	}
	if props, err := s.Stat(p); err != nil {
		return ArchiveInfo{}, err
	} else if props.Size > archLimit {
		return ArchiveInfo{}, remote.ErrTooLarge
	}
	tmp, err := os.CreateTemp("", "drag-archive-*")
	if err != nil {
		return ArchiveInfo{}, err
	}
	tmp.Close()
	if err := s.Download(p, tmp.Name(), func(int64, int64) {}); err != nil {
		os.Remove(tmp.Name())
		return ArchiveInfo{}, err
	}
	entries, err := archive.List(tmp.Name(), path.Base(p))
	if err != nil {
		os.Remove(tmp.Name())
		return ArchiveInfo{}, err
	}
	a.mu.Lock()
	id := a.newID("a")
	a.mu.Unlock()
	fs := a.files()
	fs.mu.Lock()
	fs.archives[id] = archiveFile{local: tmp.Name(), name: path.Base(p)}
	fs.mu.Unlock()
	return ArchiveInfo{ID: id, Entries: entries}, nil
}

// ArchiveRead は圧縮ファイルの中の1件を文字列で返す。バイナリや UTF-8 でないものは断る
func (a *App) ArchiveRead(id, entry string) (string, error) {
	fs := a.files()
	fs.mu.Lock()
	af, ok := fs.archives[id]
	fs.mu.Unlock()
	if !ok {
		return "", errors.New("閉じた圧縮ファイルです")
	}
	b, err := archive.Read(af.local, af.name, entry, textLimit)
	if err != nil {
		return "", err
	}
	if err := remote.CheckText(b); err != nil {
		return "", err
	}
	return string(b), nil
}

func (a *App) ArchiveClose(id string) {
	fs := a.files()
	fs.mu.Lock()
	af, ok := fs.archives[id]
	delete(fs.archives, id)
	fs.mu.Unlock()
	if ok {
		os.Remove(af.local)
	}
}

// closeFiles は終わるときに、ログの追従を止めて一時ファイルを消す
func (a *App) closeFiles() {
	fs := a.files()
	fs.mu.Lock()
	defer fs.mu.Unlock()
	for _, stop := range fs.follows {
		stop()
	}
	for _, af := range fs.archives {
		os.Remove(af.local)
	}
}
