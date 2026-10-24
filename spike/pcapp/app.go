package main

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"
)

// App は PC アプリの試作。検証用サーバーに SSH でつなぎ、
// PTY 付きのシェルを1つ開いて xterm.js に流す
type App struct {
	ctx  context.Context
	sess *ssh.Session
	in   io.Writer
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Connect は検証用サーバーにつなぎ、シェルを開く。失敗したらエラーの文を返す。
// 接続先は DRAG_ADDR、DRAG_USER、DRAG_PASS で変えられる。
// DRAG_OUTLOG にファイルを指定すると、シェルの出力をそのまま書き足す
func (a *App) Connect(cols, rows int) string {
	sc, err := ssh.Dial("tcp", env("DRAG_ADDR", "127.0.0.1:2222"), &ssh.ClientConfig{
		User:            env("DRAG_USER", "asuta"),
		Auth:            []ssh.AuthMethod{ssh.Password(env("DRAG_PASS", "drag"))},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		return err.Error()
	}
	s, err := sc.NewSession()
	if err != nil {
		return err.Error()
	}
	// 検証用サーバーには既定のロケールが無いので、日本語を打てるように UTF-8 にする
	s.Setenv("LANG", "C.UTF-8")
	if err := s.RequestPty("xterm-256color", rows, cols, nil); err != nil {
		return err.Error()
	}
	in, _ := s.StdinPipe()
	out, _ := s.StdoutPipe()
	if err := s.Shell(); err != nil {
		return err.Error()
	}
	a.sess, a.in = s, in

	var log io.Writer = io.Discard
	if p := os.Getenv("DRAG_OUTLOG"); p != "" {
		if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			log = f
		}
	}
	go func() {
		b := make([]byte, 32*1024)
		for {
			n, err := out.Read(b)
			if n > 0 {
				log.Write(b[:n])
				// UTF-8 の途中で切れたバイト列も壊さずに渡すため、base64 にする
				runtime.EventsEmit(a.ctx, "out", base64.StdEncoding.EncodeToString(b[:n]))
			}
			if err != nil {
				runtime.EventsEmit(a.ctx, "closed", err.Error())
				return
			}
		}
	}()
	return ""
}

// Input は xterm.js の onData をそのまま送る
func (a *App) Input(data string) {
	if a.in != nil {
		io.WriteString(a.in, data)
	}
}

func (a *App) Resize(cols, rows int) {
	if a.sess != nil {
		a.sess.WindowChange(rows, cols)
	}
}
