package main

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/asutarisucu/DRAG/app/internal/remote"
)

// App は画面から呼ばれる窓口。サーバーとシェルを番号で持つ
type App struct {
	ctx context.Context

	mu      sync.Mutex
	next    int
	servers map[string]*remote.Server
	shells  map[string]*remote.Shell
}

func NewApp() *App {
	return &App{servers: map[string]*remote.Server{}, shells: map[string]*remote.Shell{}}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range a.servers {
		s.Close()
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
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
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
	ID      string   `json:"id"`
	Home    string   `json:"home"`
	Error   string   `json:"error"`
	HostKey *HostKey `json:"hostKey"`
}

// Connect はサーバーにつなぐ。鍵が未登録なら HostKey を返すので、画面で確かめてから Trust を付けて呼び直す
func (a *App) Connect(req ConnectRequest) ConnectResult {
	if req.Port == 0 {
		req.Port = 22
	}
	s, err := remote.Dial(remote.Config{Host: req.Host, Port: req.Port, User: req.User, Password: req.Password, KeyFile: req.KeyFile},
		filepath.Join(configDir(), "known_hosts"), req.Trust)
	if err != nil {
		var he *remote.HostKeyError
		if errors.As(err, &he) {
			return ConnectResult{Error: he.Error(), HostKey: &HostKey{Fingerprint: he.Fingerprint, KeyType: he.KeyType, Mismatch: he.Mismatch}}
		}
		return ConnectResult{Error: err.Error()}
	}
	home, err := s.Home()
	if err != nil {
		s.Close()
		return ConnectResult{Error: err.Error()}
	}
	a.mu.Lock()
	id := a.newID("s")
	a.servers[id] = s
	a.mu.Unlock()
	go func() {
		s.Wait()
		runtime.EventsEmit(a.ctx, "server:closed", id)
	}()
	return ConnectResult{ID: id, Home: home}
}

func (a *App) server(id string) (*remote.Server, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.servers[id]
	if !ok {
		return nil, errors.New("サーバーにつながっていません")
	}
	return s, nil
}

func (a *App) Disconnect(id string) {
	a.mu.Lock()
	s := a.servers[id]
	delete(a.servers, id)
	a.mu.Unlock()
	if s != nil {
		s.Close()
	}
}

func (a *App) ListDir(serverID, dir string) ([]remote.Entry, error) {
	s, err := a.server(serverID)
	if err != nil {
		return nil, err
	}
	return s.ListDir(dir)
}

// OpenShell はシェルを開き、その番号を返す。
// 出力は "shell:out" (番号, base64)、終わったら "shell:exit" (番号) のイベントで送る
func (a *App) OpenShell(serverID string, cols, rows int, cwd string) (string, error) {
	s, err := a.server(serverID)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	id := a.newID("t")
	a.mu.Unlock()
	sh, err := s.OpenShell(cols, rows, cwd,
		// UTF-8 の途中で切れたバイト列も壊さずに渡すため、base64 にする
		func(p []byte) { runtime.EventsEmit(a.ctx, "shell:out", id, base64.StdEncoding.EncodeToString(p)) },
		func() {
			a.mu.Lock()
			delete(a.shells, id)
			a.mu.Unlock()
			runtime.EventsEmit(a.ctx, "shell:exit", id)
		})
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.shells[id] = sh
	a.mu.Unlock()
	return id, nil
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
