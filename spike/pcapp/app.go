package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"
)

// App は PC アプリの試作。検証用サーバーに SSH でつなぎ、
// エージェントのセッションを1つ開いて xterm.js に流す
type App struct {
	ctx context.Context
	sc  *ssh.Client
	ws  *websocket.Conn
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

// Connect は検証用サーバーにつなぎ、セッションを開く。失敗したらエラーの文を返す。
// 接続先は DRAG_ADDR、DRAG_USER、DRAG_PASS で変えられる
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
	a.sc = sc
	home, err := a.run("echo $HOME")
	if err != nil {
		return err.Error()
	}
	sock := strings.TrimSpace(home) + "/.drag/agent.sock"
	hc := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return sc.DialContext(ctx, "unix", sock)
		},
	}}

	// エージェントが応答しなければ、置いてある実行ファイルを起動する
	if _, err := a.get(hc, "/version"); err != nil {
		a.run("setsid -f ~/.drag/bin/drag agent </dev/null >/dev/null 2>&1")
		for i := 0; ; i++ {
			if _, err = a.get(hc, "/version"); err == nil {
				break
			}
			if i == 50 {
				return "エージェントが起動しない: " + err.Error()
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	res, err := hc.Post("http://drag/sessions", "", nil)
	if err != nil {
		return err.Error()
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	id := strings.TrimSpace(string(b))

	ws, _, err := websocket.Dial(a.ctx, "ws://drag/sessions/"+id, &websocket.DialOptions{HTTPClient: hc})
	if err != nil {
		return err.Error()
	}
	ws.SetReadLimit(-1)
	a.ws = ws
	a.Resize(cols, rows)

	go func() {
		for {
			_, p, err := ws.Read(a.ctx)
			if err != nil {
				runtime.EventsEmit(a.ctx, "closed", err.Error())
				return
			}
			// UTF-8 の途中で切れたバイト列も壊さずに渡すため、base64 にする
			runtime.EventsEmit(a.ctx, "out", base64.StdEncoding.EncodeToString(p))
		}
	}()
	return ""
}

// Input は xterm.js の onData をそのまま送る
func (a *App) Input(data string) {
	if a.ws != nil {
		a.ws.Write(a.ctx, websocket.MessageBinary, []byte(data))
	}
}

func (a *App) Resize(cols, rows int) {
	if a.ws != nil {
		a.ws.Write(a.ctx, websocket.MessageText, []byte(fmt.Sprintf(`{"cols":%d,"rows":%d}`, cols, rows)))
	}
}

func (a *App) run(cmd string) (string, error) {
	s, err := a.sc.NewSession()
	if err != nil {
		return "", err
	}
	defer s.Close()
	out, err := s.CombinedOutput(cmd)
	return string(out), err
}

func (a *App) get(hc *http.Client, path string) (string, error) {
	res, err := hc.Get("http://drag" + path)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return string(b), err
}
