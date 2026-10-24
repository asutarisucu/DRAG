// Package phone は、スマホのブラウザから PC アプリにつなぐ待ち受け。
// LAN に HTTPS で待ち受け、QR コードで登録した端末だけに、画面と WebSocket の窓口を出す
package phone

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// PairTTL は QR コードに入れるペアリング用のトークンが使える時間
const PairTTL = 5 * time.Minute

const cookieName = "drag-device"

// Caller は、スマホの画面からの呼び出しを PC アプリのメソッドに渡す
type Caller func(dev Device, method string, args []json.RawMessage) (any, error)

type Options struct {
	Dir    string // 証明書と端末の一覧を置くフォルダ
	Assets fs.FS  // スマホに出す画面。PC アプリと同じもの
	Call   Caller
	// Changed は、端末の登録・無効化・ログインで、一覧か履歴が変わったとき
	Changed func()
}

// Status は待ち受けの様子
type Status struct {
	Running bool   `json:"running"`
	URL     string `json:"url"` // https://192.168.1.20:8443
	Error   string `json:"error"`
}

type Server struct {
	opts Options
	devs *devices

	mu     sync.Mutex
	srv    *http.Server
	status Status
	// 使えるペアリング用のトークンと期限。1回使うか期限が切れたら消す。
	// 作り直しが続けて来ても、出ている QR コードがどれも使えるよう、いくつか持つ
	pairs   map[string]time.Time
	clients map[*client]struct{}
}

// client は WebSocket でつないでいる画面1つ。書き込みは out を通して1つのゴルーチンから行い、順番を保つ
type client struct {
	dev  Device
	ws   *websocket.Conn
	out  chan []byte
	done chan struct{}
	once sync.Once
}

func (c *client) send(b []byte) {
	select {
	case c.out <- b:
	case <-c.done:
	default:
		// 読み切れないほど遅れている画面は切る。つなぎ直せば続きから出る
		c.close()
	}
}

func (c *client) close() {
	c.once.Do(func() {
		close(c.done)
		c.ws.CloseNow()
	})
}

func New(opts Options) *Server {
	return &Server{opts: opts, devs: openDevices(opts.Dir), clients: map[*client]struct{}{}, pairs: map[string]time.Time{}}
}

// Start は addr (":8443" など) で待ち受ける。動いていれば止めてから始める
func (s *Server) Start(addr string) error {
	s.Stop()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	ip := lanIP()
	if h := net.ParseIP(host); h != nil && h.IsLoopback() {
		ip = h.To4()
	}
	cert, err := loadCert(s.opts.Dir, ip)
	if err != nil {
		return s.fail(fmt.Errorf("証明書を用意できません: %w", err))
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return s.fail(err)
	}
	srv := &http.Server{
		Handler:           s.routes(),
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}},
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          log.New(discard{}, "", 0),
	}
	u := "https://127.0.0.1:" + port
	if ip != nil {
		u = "https://" + ip.String() + ":" + port
	}
	s.mu.Lock()
	s.srv = srv
	s.status = Status{Running: true, URL: u}
	s.mu.Unlock()
	go srv.ServeTLS(ln, "", "")
	return nil
}

// discard は、証明書を受け入れる前のスマホが毎回出す TLS のエラーを捨てる
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func (s *Server) fail(err error) error {
	s.mu.Lock()
	s.status = Status{Error: err.Error()}
	s.mu.Unlock()
	return err
}

// Stop は待ち受けを止め、つないでいる画面を切る
func (s *Server) Stop() {
	s.mu.Lock()
	srv := s.srv
	s.srv = nil
	s.status = Status{}
	clear(s.pairs)
	clients := s.clients
	s.clients = map[*client]struct{}{}
	s.mu.Unlock()
	for c := range clients {
		c.close()
	}
	if srv != nil {
		srv.Close()
	}
}

func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// pairLimit は同時に使えるペアリング用のトークンの数。超えたら古いものから消す
const pairLimit = 5

// NewPairURL は、新しいペアリング用のトークンを入れた URL を返す
func (s *Server) NewPairURL() (string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.status.Running {
		return "", time.Time{}, errors.New("スマホの待ち受けが止まっています")
	}
	now := time.Now()
	for t, exp := range s.pairs {
		if now.After(exp) {
			delete(s.pairs, t)
		}
	}
	for len(s.pairs) >= pairLimit {
		var oldest string
		for t, exp := range s.pairs {
			if oldest == "" || exp.Before(s.pairs[oldest]) {
				oldest = t
			}
		}
		delete(s.pairs, oldest)
	}
	t := rand.Text()
	s.pairs[t] = now.Add(PairTTL)
	return s.status.URL + "/pair?t=" + t, s.pairs[t], nil
}

// takePair は、トークンが合っていて期限内なら使用済みにして true を返す
func (s *Server) takePair(t string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for p, exp := range s.pairs {
		if subtle.ConstantTimeCompare([]byte(t), []byte(p)) == 1 {
			delete(s.pairs, p)
			return time.Now().Before(exp)
		}
	}
	return false
}

// Devices は登録した端末。資格情報のハッシュは外す
func (s *Server) Devices() []Device {
	list := s.devs.all()
	for i := range list {
		list[i].Hash = ""
	}
	return list
}

func (s *Server) History() []Login { return s.devs.history() }

// Revoke は端末を無効にし、その端末の画面を切る
func (s *Server) Revoke(id string) error {
	if err := s.devs.remove(id); err != nil {
		return err
	}
	s.mu.Lock()
	var drop []*client
	for c := range s.clients {
		if c.dev.ID == id {
			drop = append(drop, c)
		}
	}
	s.mu.Unlock()
	for _, c := range drop {
		c.close()
	}
	s.changed()
	return nil
}

func (s *Server) changed() {
	if s.opts.Changed != nil {
		s.opts.Changed()
	}
}

type eventMsg struct {
	Event string `json:"ev"`
	Args  []any  `json:"a"`
}

// Broadcast は、つないでいる全部の画面にイベントを送る
func (s *Server) Broadcast(event string, args ...any) {
	s.BroadcastExcept("", event, args...)
}

// BroadcastExcept は、except の端末を除いた画面にイベントを送る
func (s *Server) BroadcastExcept(except, event string, args ...any) {
	if s == nil {
		return
	}
	b, err := json.Marshal(eventMsg{event, args})
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		if except == "" || c.dev.ID != except {
			c.send(b)
		}
	}
}

func clientIP(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

// device は cookie の資格情報から端末を探す
func (s *Server) device(r *http.Request) (Device, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return Device{}, false
	}
	return s.devs.check(c.Value)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	assets := http.FileServerFS(s.opts.Assets)

	mux.HandleFunc("GET /pair", func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !s.takePair(r.URL.Query().Get("t")) {
			s.devs.log(Login{Time: time.Now(), Kind: "pair-failed", IP: ip, Route: "lan"})
			s.changed()
			page(w, r, http.StatusGone, pageExpired)
			return
		}
		cred, dev, err := s.devs.add(deviceName(r.UserAgent()), "lan", ip)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// ブラウザが持てる最長の 400 日にする。なくしたときは PC アプリから無効にする
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: cred, Path: "/", MaxAge: 400 * 24 * 3600, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		s.devs.log(Login{Time: time.Now(), Kind: "pair", Device: dev.Name, IP: ip, Route: "lan"})
		s.changed()
		http.Redirect(w, r, "/?paired=1", http.StatusSeeOther)
	})

	mux.HandleFunc("GET /api/ws", func(w http.ResponseWriter, r *http.Request) {
		// cookie は別のサイトからの WebSocket にも付くので、接続元がこのサイトかを確かめる
		origin, err := url.Parse(r.Header.Get("Origin"))
		if err != nil || origin.Host == "" || !strings.EqualFold(origin.Host, r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		dev, ok := s.device(r)
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		s.serveWS(dev, ws, clientIP(r))
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.device(r); !ok {
			if r.URL.Path == "/" {
				s.devs.log(Login{Time: time.Now(), Kind: "denied", IP: clientIP(r), Route: "lan"})
				s.changed()
			}
			page(w, r, http.StatusForbidden, pageNotRegistered)
			return
		}
		assets.ServeHTTP(w, r)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}

type callMsg struct {
	ID     int64             `json:"id"`
	Method string            `json:"m"`
	Args   []json.RawMessage `json:"a"`
}

type replyMsg struct {
	ID     int64  `json:"id"`
	Result any    `json:"r"`
	Error  string `json:"e,omitempty"`
}

func (s *Server) serveWS(dev Device, ws *websocket.Conn, ip string) {
	ws.SetReadLimit(32 << 20)
	c := &client{dev: dev, ws: ws, out: make(chan []byte, 4096), done: make(chan struct{})}
	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()
	s.devs.seen(dev.ID, ip)
	s.devs.log(Login{Time: time.Now(), Kind: "login", Device: dev.Name, IP: ip, Route: dev.Route})
	s.changed()
	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
		c.close()
	}()

	ctx := context.Background()
	go func() {
		for {
			select {
			case b := <-c.out:
				wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := ws.Write(wctx, websocket.MessageText, b)
				cancel()
				if err != nil {
					c.close()
					return
				}
			case <-c.done:
				return
			}
		}
	}()

	for {
		_, b, err := ws.Read(ctx)
		if err != nil {
			return
		}
		var m callMsg
		if json.Unmarshal(b, &m) != nil || m.Method == "" {
			continue
		}
		// PC アプリの画面と同じく、呼び出しは並行して動かす。順番が要る入力は画面の側で1つずつ送る
		go func() {
			res, err := s.opts.Call(dev, m.Method, m.Args)
			rep := replyMsg{ID: m.ID, Result: res}
			if err != nil {
				rep = replyMsg{ID: m.ID, Error: err.Error()}
			}
			out, merr := json.Marshal(rep)
			if merr != nil {
				out, _ = json.Marshal(replyMsg{ID: m.ID, Error: merr.Error()})
			}
			c.send(out)
		}()
	}
}
