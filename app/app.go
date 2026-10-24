package main

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/asutarisucu/DRAG/app/internal/phone"
	"github.com/asutarisucu/DRAG/app/internal/remote"
	"github.com/asutarisucu/DRAG/app/internal/store"
)

// App は画面から呼ばれる窓口。サーバーとシェルを番号で持つ。
// PC の画面とスマホの画面で core を共有し、dev で呼んだ端末を分ける
type App struct {
	*core
	dev string // 呼んだ端末。PC の画面なら空
}

type core struct {
	ctx   context.Context
	saved *store.Servers
	phone *phone.Server
	tray  *tray // 通知領域に置かないときは nil

	mu      sync.Mutex
	next    int
	servers map[string]*conn
	shells  map[string]*openShell
	fs      *fileState
}

// openShell は開いているシェル1つと、それを開いた端末。ほかの端末から続きを操作するときに一覧に出す
type openShell struct {
	*remote.Shell
	server string
	dev    string
}

// conn はつないでいるサーバー1台。再接続に使うので、接続の設定 (パスワードを含む) をメモリーにだけ持つ
type conn struct {
	req    ConnectRequest
	server *remote.Server
	info   *remote.Info
	// つなぎ直しは PC とスマホの両方から来るので、1つずつにする
	redial sync.Mutex
}

func NewApp() *App {
	return &App{core: &core{
		saved:   store.NewServers(configDir()),
		servers: map[string]*conn{},
		shells:  map[string]*openShell{},
	}}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.startPhone()
	a.startTray()
}

func (a *App) shutdown(ctx context.Context) {
	a.stopTray()
	a.phone.Stop()
	a.closeFiles()
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
	return filepath.Join(d, configName)
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
	Sudo    bool         `json:"sudo"` // つなぎ直したあとも sudo モードが ON か
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
	s.Close()
	a.mu.Lock()
	c := a.servers[id]
	current := c != nil && c.server == s
	a.mu.Unlock()
	if current {
		a.emit("server:closed", id)
	}
}

// emit は PC の画面とスマホの画面の両方にイベントを送る
func (a *App) emit(name string, args ...any) {
	runtime.EventsEmit(a.ctx, name, args...)
	a.phone.Broadcast(name, args...)
}

// emitOthers は、呼んだ端末を除いた画面にイベントを送る
func (a *App) emitOthers(name string, args ...any) {
	if a.dev != "" {
		runtime.EventsEmit(a.ctx, name, args...)
	}
	a.phone.BroadcastExcept(a.dev, name, args...)
}

// ServerInfo はつないでいるサーバー1台。スマホの画面が開いたときに、PC でつないだサーバーを並べるのに使う
type ServerInfo struct {
	ID   string       `json:"id"`
	Name string       `json:"name"`
	User string       `json:"user"`
	Host string       `json:"host"`
	Home string       `json:"home"`
	Info *remote.Info `json:"info"`
}

func serverInfo(id string, c *conn) ServerInfo {
	home, _ := c.server.Home()
	return ServerInfo{ID: id, Name: c.req.Name, User: c.req.User, Host: c.req.Host, Home: home, Info: c.info}
}

// Servers はつないでいるサーバーを、つないだ順に返す
func (a *App) Servers() []ServerInfo {
	a.mu.Lock()
	conns := maps.Clone(a.servers)
	a.mu.Unlock()
	out := []ServerInfo{}
	for _, id := range slices.SortedFunc(maps.Keys(conns), func(x, y string) int { return cmp.Compare(idNum(x), idNum(y)) }) {
		out = append(out, serverInfo(id, conns[id]))
	}
	return out
}

// idNum は "s12" のような番号の数の部分
func idNum(id string) int {
	n, _ := strconv.Atoi(strings.TrimLeft(id, "abcdefghijklmnopqrstuvwxyz"))
	return n
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
	c := &conn{req: req, server: s}
	a.servers[id] = c
	a.mu.Unlock()
	go a.watch(id, s)
	// ほかの端末の画面にもタブを足す。開いた端末は、接続の手順を見せてから自分で足す
	a.emitOthers("server:opened", serverInfo(id, c))
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
	info, err := s.Probe()
	if err == nil {
		a.mu.Lock()
		if c := a.servers[id]; c != nil {
			c.info = &info
		}
		a.mu.Unlock()
	}
	return info, err
}

// Reconnect は、切れたサーバーに同じ設定でつなぎ直す。番号はそのまま使う。
// ほかの端末が先につなぎ直していれば、その接続を使う
func (a *App) Reconnect(id string) ConnectResult {
	a.mu.Lock()
	c := a.servers[id]
	a.mu.Unlock()
	if c == nil {
		return ConnectResult{Error: "サーバーにつながっていません"}
	}
	c.redial.Lock()
	defer c.redial.Unlock()
	if !c.server.Closed() {
		home, _ := c.server.Home()
		return ConnectResult{ID: id, Home: home, Info: c.info, Sudo: c.server.As(a.dev).Sudo()}
	}
	req := c.req
	req.Trust = ""
	s, r := a.dial(req)
	if s == nil {
		return r
	}
	// sudo モードは、切れる前に ON だった端末ごとに、同じパスワードで ON に戻す。パスワードが変わっていたら OFF のまま
	for dev, pw := range c.server.SudoPasswords() {
		ok := s.As(dev).EnableSudo(pw) == nil
		if dev == a.dev {
			r.Sudo = ok
		}
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

// EnableSudo はサーバーの sudo モードを ON にする。パスワードは PC アプリのメモリーにだけ持つ
func (a *App) EnableSudo(id, password string) error {
	s, err := a.server(id)
	if err != nil {
		return err
	}
	return s.EnableSudo(password)
}

// DisableSudo はサーバーの sudo モードを OFF にする
func (a *App) DisableSudo(id string) error {
	s, err := a.server(id)
	if err != nil {
		return err
	}
	s.DisableSudo()
	return nil
}

func (a *App) server(id string) (*remote.Server, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.servers[id]
	if !ok {
		return nil, errors.New("サーバーにつながっていません")
	}
	return c.server.As(a.dev), nil
}

func (a *App) Disconnect(id string) {
	a.mu.Lock()
	c := a.servers[id]
	delete(a.servers, id)
	a.mu.Unlock()
	if c != nil {
		c.server.Close()
		a.emitOthers("server:removed", id)
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
			a.emit("shell:out", id, base64.StdEncoding.EncodeToString(p), end)
		},
		Foreground: func(cmdline string) { a.emit("shell:fg", id, cmdline) },
		Exit: func() {
			a.mu.Lock()
			delete(a.shells, id)
			a.mu.Unlock()
			a.emit("shell:exit", id)
		},
	})
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.shells[id] = &openShell{Shell: sh, server: serverID, dev: a.dev}
	a.mu.Unlock()
	return id, nil
}

// ShellInfo は、ほかの端末で開いているシェル1つ。Owner はそのシェルを開いた端末の名前
type ShellInfo struct {
	ID    string `json:"id"`
	Owner string `json:"owner"`
	Fg    string `json:"fg"`
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
}

// OtherShells は、serverID で、呼んだ端末とは別の端末が開いているシェル。
// PC で開いていたターミナルを、スマホから続けて操作するのに使う
func (a *App) OtherShells(serverID string) []ShellInfo {
	names := map[string]string{"": "PC"}
	for _, d := range a.phone.Devices() {
		names[d.ID] = d.Name
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []ShellInfo{}
	for _, id := range slices.SortedFunc(maps.Keys(a.shells), func(x, y string) int { return cmp.Compare(idNum(x), idNum(y)) }) {
		sh := a.shells[id]
		if sh.server != serverID || sh.dev == a.dev {
			continue
		}
		cols, rows := sh.Size()
		out = append(out, ShellInfo{ID: id, Owner: names[sh.dev], Fg: sh.Foreground(), Cols: cols, Rows: rows})
	}
	return out
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
		a.emit("transfer:progress", key, done, total)
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

// Sessions は screen と tmux のセッションの一覧
func (a *App) Sessions(serverID string) ([]remote.Session, error) {
	s, err := a.server(serverID)
	if err != nil {
		return nil, err
	}
	return s.Sessions()
}

func (a *App) shell(id string) *remote.Shell {
	a.mu.Lock()
	defer a.mu.Unlock()
	if sh := a.shells[id]; sh != nil {
		return sh.Shell
	}
	return nil
}

func (a *App) ShellInput(id, data string) {
	if sh := a.shell(id); sh != nil {
		sh.Write([]byte(data))
	}
}

// ShellResize は PTY の大きさを変え、"shell:size" (番号, 桁数, 行数) を送る。
// ほかの端末で同じシェルを見ている画面は、この大きさに合わせて描く
func (a *App) ShellResize(id string, cols, rows int) {
	if sh := a.shell(id); sh != nil {
		sh.Resize(cols, rows)
		a.emit("shell:size", id, cols, rows)
	}
}

func (a *App) CloseShell(id string) {
	if sh := a.shell(id); sh != nil {
		sh.Close()
	}
}
