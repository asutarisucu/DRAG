package main

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/asutarisucu/DRAG/app/internal/remote"
	"github.com/asutarisucu/DRAG/app/internal/store"
)

// App は画面から呼ばれる窓口。サーバーとシェルを番号で持つ
type App struct {
	ctx   context.Context
	saved *store.Servers

	mu      sync.Mutex
	next    int
	servers map[string]*conn
	shells  map[string]*remote.Shell
}

// conn はつないでいるサーバー1台。再接続に使うので、接続の設定 (パスワードを含む) をメモリーにだけ持つ
type conn struct {
	req    ConnectRequest
	server *remote.Server
}

func NewApp() *App {
	return &App{
		saved:   store.NewServers(configDir()),
		servers: map[string]*conn{},
		shells:  map[string]*remote.Shell{},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, c := range a.servers {
		c.server.Close()
	}
}

func (a *App) newID(prefix string) string {
	a.next++
	return prefix + strconv.Itoa(a.next)
}

// configDir は DRAG の設定と記録を置くフォルダ。Windows では %AppData%\DRAG
func configDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = "."
	}
	return filepath.Join(d, "DRAG")
}

type ConnectRequest struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Auth     string `json:"auth"` // "pass" か "key"
	Password string `json:"password"`
	KeyFile  string `json:"keyFile"`
	// 利用者が画面で確かめたサーバーの鍵の指紋。未登録の鍵を登録するときに渡す
	Trust string `json:"trust"`
}

type HostKey struct {
	Fingerprint string `json:"fingerprint"`
	KeyType     string `json:"keyType"`
	Mismatch    bool   `json:"mismatch"`
}

type ConnectResult struct {
	ID      string       `json:"id"`
	Home    string       `json:"home"`
	Info    *remote.Info `json:"info"`
	Error   string       `json:"error"`
	HostKey *HostKey     `json:"hostKey"`
}

func (a *App) dial(req ConnectRequest) (*remote.Server, ConnectResult) {
	if req.Port == 0 {
		req.Port = 22
	}
	keyFile := ""
	if req.Auth == "key" {
		keyFile = req.KeyFile
	}
	s, err := remote.Dial(remote.Config{Host: req.Host, Port: req.Port, User: req.User, Password: req.Password, KeyFile: keyFile},
		filepath.Join(configDir(), "known_hosts"), req.Trust)
	if err != nil {
		var he *remote.HostKeyError
		if errors.As(err, &he) {
			return nil, ConnectResult{Error: he.Error(), HostKey: &HostKey{Fingerprint: he.Fingerprint, KeyType: he.KeyType, Mismatch: he.Mismatch}}
		}
		return nil, ConnectResult{Error: err.Error()}
	}
	home, err := s.Home()
	if err != nil {
		s.Close()
		return nil, ConnectResult{Error: err.Error()}
	}
	return s, ConnectResult{Home: home}
}

// watch は接続が切れたら "server:closed" を送る。切断や再接続で入れ替えたあとの古い接続では送らない
func (a *App) watch(id string, s *remote.Server) {
	s.Wait(15*time.Second, 10*time.Second)
	a.mu.Lock()
	c := a.servers[id]
	current := c != nil && c.server == s
	a.mu.Unlock()
	if current {
		runtime.EventsEmit(a.ctx, "server:closed", id)
	}
}

// Connect はサーバーにつなぎ、保存済みのサーバーに書く。
// 鍵が未登録なら HostKey を返すので、画面で確かめてから Trust を付けて呼び直す
func (a *App) Connect(req ConnectRequest) ConnectResult {
	s, r := a.dial(req)
	if s == nil {
		return r
	}
	a.mu.Lock()
	id := a.newID("s")
	a.servers[id] = &conn{req: req, server: s}
	a.mu.Unlock()
	go a.watch(id, s)
	if _, err := a.saved.Put(store.Server{Name: req.Name, Host: req.Host, Port: req.Port, User: req.User, Auth: req.Auth, KeyFile: req.KeyFile}); err != nil {
		runtime.LogWarningf(a.ctx, "保存済みのサーバーに書けません: %v", err)
	}
	r.ID = id
	return r
}

// Probe は、つないだサーバーの OS と使える機能を調べる
func (a *App) Probe(id string) (remote.Info, error) {
	s, err := a.server(id)
	if err != nil {
		return remote.Info{}, err
	}
	return s.Probe()
}

// Reconnect は、切れたサーバーに同じ設定でつなぎ直す。番号はそのまま使う
func (a *App) Reconnect(id string) ConnectResult {
	a.mu.Lock()
	c := a.servers[id]
	a.mu.Unlock()
	if c == nil {
		return ConnectResult{Error: "サーバーにつながっていません"}
	}
	req := c.req
	req.Trust = ""
	s, r := a.dial(req)
	if s == nil {
		return r
	}
	a.mu.Lock()
	old := c.server
	c.server = s
	a.mu.Unlock()
	old.Close()
	go a.watch(id, s)
	r.ID = id
	return r
}

func (a *App) server(id string) (*remote.Server, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.servers[id]
	if !ok {
		return nil, errors.New("サーバーにつながっていません")
	}
	return c.server, nil
}

func (a *App) Disconnect(id string) {
	a.mu.Lock()
	c := a.servers[id]
	delete(a.servers, id)
	a.mu.Unlock()
	if c != nil {
		c.server.Close()
	}
}

func (a *App) SavedServers() ([]store.Server, error) {
	return a.saved.List()
}

func (a *App) RemoveSavedServer(id string) error {
	return a.saved.Remove(id)
}

func (a *App) ListDir(serverID, dir string) ([]remote.Entry, error) {
	s, err := a.server(serverID)
	if err != nil {
		return nil, err
	}
	return s.ListDir(dir)
}

// OpenShell はシェルを開き、その番号を返す。
// 出力は "shell:out" (番号, base64, 通算の位置)、前面のコマンドが変わったら "shell:fg" (番号, コマンド)、
// 終わったら "shell:exit" (番号) のイベントで送る
func (a *App) OpenShell(serverID string, cols, rows int, cwd string) (string, error) {
	s, err := a.server(serverID)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	id := a.newID("t")
	a.mu.Unlock()
	sh, err := s.OpenShell(cols, rows, cwd, remote.ShellEvents{
		// UTF-8 の途中で切れたバイト列も壊さずに渡すため、base64 にする
		Output: func(p []byte, end int64) {
			runtime.EventsEmit(a.ctx, "shell:out", id, base64.StdEncoding.EncodeToString(p), end)
		},
		Foreground: func(cmdline string) { runtime.EventsEmit(a.ctx, "shell:fg", id, cmdline) },
		Exit: func() {
			a.mu.Lock()
			delete(a.shells, id)
			a.mu.Unlock()
			runtime.EventsEmit(a.ctx, "shell:exit", id)
		},
	})
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.shells[id] = sh
	a.mu.Unlock()
	return id, nil
}

type Replay struct {
	Data string `json:"data"` // base64
	End  int64  `json:"end"`
}

// ShellReplay は、残しておいたシェルの出力を返す。ウィンドウへ移したときに、新しい画面に描き直すのに使う。
// fromIdle なら、移したコマンドを打った行から後ろだけを返す
func (a *App) ShellReplay(id string, fromIdle bool) (Replay, error) {
	sh := a.shell(id)
	if sh == nil {
		return Replay{}, errors.New("シェルが終わっています")
	}
	data, end := sh.Replay(fromIdle)
	return Replay{Data: base64.StdEncoding.EncodeToString(data), End: end}, nil
}

// ShellCwd はシェルの作業中のフォルダ。ウィンドウへ移したあと、同じフォルダで新しいシェルを開くのに使う
func (a *App) ShellCwd(id string) (string, error) {
	sh := a.shell(id)
	if sh == nil {
		return "", errors.New("シェルが終わっています")
	}
	return sh.Cwd()
}

// NewEntry は dir の中に空のファイルかフォルダを作る。name が使われていれば番号を足し、作ったパスを返す
func (a *App) NewEntry(serverID, dir, name string, isDir bool) (string, error) {
	s, err := a.server(serverID)
	if err != nil {
		return "", err
	}
	p := path.Join(dir, s.UniqueName(dir, name, false))
	if isDir {
		return p, s.Mkdir(p)
	}
	return p, s.CreateFile(p)
}

// RenameEntry は名前を変え、新しいパスを返す
func (a *App) RenameEntry(serverID, from, name string) (string, error) {
	if name == "" || strings.Contains(name, "/") {
		return "", errors.New("名前に / は使えません")
	}
	s, err := a.server(serverID)
	if err != nil {
		return "", err
	}
	to := path.Join(path.Dir(from), name)
	return to, s.Rename(from, to)
}

// PasteEntry は from を dir の中へ複製するか移す。同じ名前があれば名前を変えて置き、置いたパスを返す
func (a *App) PasteEntry(serverID, from, dir string, move bool) (string, error) {
	if dir == from || strings.HasPrefix(dir, from+"/") {
		return "", errors.New("フォルダをその中へは移せません")
	}
	s, err := a.server(serverID)
	if err != nil {
		return "", err
	}
	if move && path.Dir(from) == dir {
		return from, nil
	}
	to := path.Join(dir, s.UniqueName(dir, path.Base(from), !move))
	if move {
		return to, s.Rename(from, to)
	}
	return to, s.Copy(from, to)
}

func (a *App) TrashEntry(serverID, p string) (remote.TrashItem, error) {
	s, err := a.server(serverID)
	if err != nil {
		return remote.TrashItem{}, err
	}
	return s.Trash(p)
}

func (a *App) RestoreEntry(serverID string, item remote.TrashItem) (string, error) {
	s, err := a.server(serverID)
	if err != nil {
		return "", err
	}
	return s.Restore(item)
}

// progress は送った量を "transfer:progress" (key, done, total) のイベントで送る。多すぎないよう 0.1 秒に1回まで
func (a *App) progress(key string) remote.Progress {
	var last time.Time
	return func(done, total int64) {
		if done < total && time.Since(last) < 100*time.Millisecond {
			return
		}
		last = time.Now()
		runtime.EventsEmit(a.ctx, "transfer:progress", key, done, total)
	}
}

// PickUploadFiles は、アップロードするファイルを PC から選ぶ窓を出す
func (a *App) PickUploadFiles() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{})
}

type LocalEntry struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
	Size int64  `json:"size"`
}

// LocalEntries は、ドロップされたり選ばれたりした PC のファイルの名前と大きさを返す
func (a *App) LocalEntries(paths []string) ([]LocalEntry, error) {
	out := make([]LocalEntry, 0, len(paths))
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		size, err := remote.LocalSize(p)
		if err != nil {
			return nil, err
		}
		out = append(out, LocalEntry{Path: p, Name: filepath.Base(p), Dir: fi.IsDir(), Size: size})
	}
	return out, nil
}

// Upload は PC のファイルかフォルダを dir へ送り、置いたパスを返す。keep なら同じ名前を避け、そうでなければ上書きする
func (a *App) Upload(serverID, local, dir string, keep bool, key string) (string, error) {
	s, err := a.server(serverID)
	if err != nil {
		return "", err
	}
	return s.Upload(local, dir, keep, a.progress(key))
}

// Download は保存先を PC で選んでもらい、そこへ書く。フォルダは zip にする。やめたら空を返す
func (a *App) Download(serverID, p, key string) (string, error) {
	s, err := a.server(serverID)
	if err != nil {
		return "", err
	}
	props, err := s.Stat(p)
	if err != nil {
		return "", err
	}
	name := path.Base(p)
	if strings.HasPrefix(props.Mode, "d") {
		name += ".zip"
	}
	local, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{DefaultFilename: name})
	if err != nil || local == "" {
		return "", err
	}
	return local, s.Download(p, local, a.progress(key))
}

func (a *App) Stat(serverID, p string) (remote.Props, error) {
	s, err := a.server(serverID)
	if err != nil {
		return remote.Props{}, err
	}
	return s.Stat(p)
}

func (a *App) shell(id string) *remote.Shell {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.shells[id]
}

func (a *App) ShellInput(id, data string) {
	if sh := a.shell(id); sh != nil {
		sh.Write([]byte(data))
	}
}

func (a *App) ShellResize(id string, cols, rows int) {
	if sh := a.shell(id); sh != nil {
		sh.Resize(cols, rows)
	}
}

func (a *App) CloseShell(id string) {
	if sh := a.shell(id); sh != nil {
		sh.Close()
	}
}
