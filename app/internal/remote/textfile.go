package remote

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	ErrTooLarge = errors.New("ファイルが大きすぎます")
	ErrBinary   = errors.New("バイナリのファイルです")
	ErrNotUTF8  = errors.New("UTF-8 ではないファイルです")
	ErrChanged  = errors.New("開いたあとにサーバーで書き換えられています")
)

// TextFile はエディターで開いたファイル
type TextFile struct {
	Text    string `json:"text"`
	ModTime int64  `json:"modTime"` // 保存するときに、開いたあとに変わっていないかを見る
}

// CheckText は、エディターで扱える中身かを見る。NUL を含むものはバイナリ、UTF-8 として読めないものは文字化けするので断る
func CheckText(b []byte) error {
	head := b
	if len(head) > 8192 {
		head = head[:8192]
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return ErrBinary
	}
	if !utf8.Valid(b) {
		return ErrNotUTF8
	}
	return nil
}

// ReadText はファイルを文字列で読む。limit を超えるものは読まない
func (s *Server) ReadText(p string, limit int64) (TextFile, error) {
	fi, err := s.sftp.Stat(p)
	if err != nil {
		return TextFile{}, err
	}
	if fi.Size() > limit {
		return TextFile{}, ErrTooLarge
	}
	b, err := s.ReadBytes(p, limit)
	if err != nil {
		return TextFile{}, err
	}
	if err := CheckText(b); err != nil {
		return TextFile{}, err
	}
	return TextFile{Text: string(b), ModTime: fi.ModTime().Unix()}, nil
}

// ReadBytes はファイルを読む。limit を超えるものは読まない
func (s *Server) ReadBytes(p string, limit int64) ([]byte, error) {
	f, err := s.sftp.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, ErrTooLarge
	}
	return b, nil
}

// WriteText はファイルを書き換え、新しい更新時刻を返す。
// 開いたあとに変わっていれば ErrChanged。force なら構わず書く。
// 持ち主と権限を変えないよう、別名で書いて置き換えるのではなく、同じファイルを切り詰めて書く
func (s *Server) WriteText(p, text string, modTime int64, force bool) (int64, error) {
	if !force {
		fi, err := s.sftp.Stat(p)
		if err != nil {
			return 0, err
		}
		if fi.ModTime().Unix() != modTime {
			return 0, ErrChanged
		}
	}
	f, err := s.sftp.OpenFile(p, os.O_WRONLY|os.O_TRUNC|os.O_CREATE)
	if err != nil {
		return 0, err
	}
	_, err = io.Copy(f, strings.NewReader(text))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	fi, err := s.sftp.Stat(p)
	if err != nil {
		return 0, err
	}
	return fi.ModTime().Unix(), nil
}

// LogChunk はログのウィンドウに出す一続きの中身。start と end はファイルの中のバイトの位置
type LogChunk struct {
	Text  string `json:"text"`
	Start int64  `json:"start"`
	End   int64  `json:"end"`
}

// ReadTail は、end より前の size バイトほどを、行の頭から読む。end が負ならファイルの末尾から。
// 大きいログは末尾から読み、上へスクロールしたら前を読み足すのに使う
func (s *Server) ReadTail(p string, end, size int64) (LogChunk, error) {
	if end < 0 {
		fi, err := s.sftp.Stat(p)
		if err != nil {
			return LogChunk{}, err
		}
		end = fi.Size()
	}
	start := max(0, end-size)
	f, err := s.sftp.Open(p)
	if err != nil {
		return LogChunk{}, err
	}
	defer f.Close()
	b := make([]byte, end-start)
	n, err := f.ReadAt(b, start)
	if err != nil && !errors.Is(err, io.EOF) {
		return LogChunk{}, err
	}
	b = b[:n]
	// 途中から読んだときは、最初の改行までを捨てて行の頭から始める
	if start > 0 {
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
			start += int64(i + 1)
		}
	}
	return LogChunk{Text: strings.ToValidUTF8(string(b), "�"), Start: start, End: start + int64(len(b))}, nil
}

// Follow は from バイト目から後ろを、追記されるたびに onData へ渡す。サーバーの tail -F を使う。
// 返した関数で止める
func (s *Server) Follow(p string, from int64, onData func([]byte), onEnd func(error)) (func(), error) {
	sess, err := s.ssh.NewSession()
	if err != nil {
		return nil, err
	}
	sess.Stdin = strings.NewReader(p + "\n")
	out, _ := sess.StdoutPipe()
	var stderr bytes.Buffer
	sess.Stderr = &stderr
	// tail -c +N は N バイト目 (1 から数える) から出す
	cmd := fmt.Sprintf(`exec /bin/sh -c 'IFS= read -r p; exec tail -c +%s -F -- "$p"'`, strconv.FormatInt(from+1, 10))
	if err := sess.Start(cmd); err != nil {
		sess.Close()
		return nil, err
	}
	go func() {
		b := make([]byte, 32*1024)
		for {
			n, err := out.Read(b)
			if n > 0 {
				onData(bytes.Clone(b[:n]))
			}
			if err != nil {
				sess.Wait()
				if msg := strings.TrimSpace(stderr.String()); msg != "" {
					onEnd(errors.New(msg))
				} else {
					onEnd(nil)
				}
				return
			}
		}
	}()
	return func() { sess.Close() }, nil
}
