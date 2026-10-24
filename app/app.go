package main

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strconv"
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
