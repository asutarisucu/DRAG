// Package phone は、スマホのブラウザから PC アプリにつなぐ待ち受け。
// LAN 内 ("lan") と直接公開 ("public") の2つの経路に HTTPS で待ち受け、
// QR コードで登録した端末だけに、画面と WebSocket の窓口を出す。端末は登録した経路でだけ入れる
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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/acme"
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
	// 直接公開だけ。証明書の期限 (Unix ミリ秒。まだ無ければ 0) と、証明書を取っている途中か
	CertUntil int64 `json:"certUntil"`
	Pending   bool  `json:"pending"`
}

const (
	RouteLAN    = "lan"
	RoutePublic = "public"
)

// listener は経路1つの待ち受け
type listener struct {
	srv    *http.Server
	status Status
	// 使えるペアリング用のトークンと期限。1回使うか期限が切れたら消す。
	// 作り直しが続けて来ても、出ている QR コードがどれも使えるよう、いくつか持つ
	pairs map[string]time.Time
	stop  func() // 直接公開の証明書を取り直すゴルーチンを止める
}

type Server struct {
	opts Options
	devs *devices

	mu      sync.Mutex
	routes  map[string]*listener
	clients map[*client]struct{}
}

// client は WebSocket でつないでいる画面1つ。書き込みは out を通して1つのゴルーチンから行い、順番を保つ
type client struct {
	route string
	dev   Device
	ws    *websocket.Conn
	out   chan []byte
	done  chan struct{}
	once  sync.Once
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
	return &Server{opts: opts, devs: openDevices(opts.Dir), clients: map[*client]struct{}{}, routes: map[string]*listener{}}
}

// Start は LAN 内の経路を addr (":8443" など) で待ち受ける。自己署名の証明書を使う。動いていれば止めてから始める
func (s *Server) Start(addr string) error {
	s.StopRoute(RouteLAN)
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
		return s.fail(RouteLAN, fmt.Errorf("証明書を用意できません: %w", err))
	}
	u := "https://127.0.0.1:" + port
	if ip != nil {
		u = "https://" + ip.String() + ":" + port
	}
	return s.listen(RouteLAN, addr, &tls.Config{Certificates: []tls.Certificate{cert}}, Status{Running: true, URL: u})
}

func (s *Server) listen(route, addr string, tc *tls.Config, st Status) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return s.fail(route, err)
	}
	srv := &http.Server{
		Handler:           s.handler(route),
		TLSConfig:         tc,
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          log.New(discard{}, "", 0),
	}
	s.mu.Lock()
	s.routes[route] = &listener{srv: srv, status: st, pairs: map[string]time.Time{}}
	s.mu.Unlock()
	go srv.ServeTLS(ln, "", "")
	return nil
}

// PublicConfig は直接公開の設定
type PublicConfig struct {
	Port int    // 待ち受けるポート。ルーターでこのポートを PC へ転送しておく
	IP   string // グローバル IP。空なら調べ、変わったら取り直す
	ACME string // ACME のディレクトリ。空なら Let's Encrypt
	// 試すときに使う。ACME のサーバーと話す HTTP のクライアントと、HTTP-01 の検証を待ち受けるポート (空なら 80)、
	// 待ち受けるホスト (空なら全部のアドレス)
	Client   *http.Client
	HTTPPort int
	Host     string
}

// StartPublic は直接公開の経路を待ち受け、IP アドレスの証明書を取る。
// 証明書は残りが半分を切るか、グローバル IP が変わったら取り直す。動いていれば止めてから始める
func (s *Server) StartPublic(cfg PublicConfig) error {
	s.StopRoute(RoutePublic)
	if cfg.ACME == "" {
		cfg.ACME = LetsEncrypt
	}
	ctx, cancel := context.WithCancel(context.Background())
	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = 80
	}
	m, err := newIPCert(ctx, cfg.ACME, filepath.Join(s.opts.Dir, "acme-account.pem"), cfg.Port, net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.HTTPPort)), cfg.Client)
	if err != nil {
		cancel()
		return s.fail(RoutePublic, fmt.Errorf("Let's Encrypt に登録できません: %w", err))
	}
	tc := &tls.Config{GetCertificate: m.getCertificate, NextProtos: []string{"http/1.1", acme.ALPNProto}}
	if err := s.listen(RoutePublic, net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)), tc, Status{Running: true, Pending: true}); err != nil {
		cancel()
		return err
	}
	s.mu.Lock()
	s.routes[RoutePublic].stop = cancel
	s.mu.Unlock()
	go s.keepCert(ctx, m, cfg)
	return nil
}

// keepCert は証明書を取り、そのあとは1時間おきに取り直すかを見る
func (s *Server) keepCert(ctx context.Context, m *ipCert, cfg PublicConfig) {
	ip := ""
	for {
		cur := cfg.IP
		if cur == "" {
			if got, err := GlobalIP(ctx); err == nil {
				cur = got
			} else if ip == "" {
				s.setPublic(func(st *Status) { st.Error = err.Error() })
			}
		}
		if cur != "" && (cur != ip || m.needRenew(time.Now())) {
			s.setPublic(func(st *Status) { st.Pending = true })
			if err := m.obtain(ctx, cur); err != nil {
				if ctx.Err() != nil {
					return
				}
				s.setPublic(func(st *Status) { st.Pending, st.Error = false, err.Error() })
			} else {
				ip = cur
				u := "https://" + cur
				if cfg.Port != 443 {
					u += ":" + strconv.Itoa(cfg.Port)
				}
				s.setPublic(func(st *Status) { st.Pending, st.Error, st.URL, st.CertUntil = false, "", u, m.until().UnixMilli() })
			}
			s.changed()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}

func (s *Server) setPublic(f func(*Status)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l := s.routes[RoutePublic]; l != nil {
		f(&l.status)
	}
}

// discard は、証明書を受け入れる前のスマホが毎回出す TLS のエラーを捨てる
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// fail は、待ち受けを始められなかった理由を、止まっている経路の様子として残す
func (s *Server) fail(route string, err error) error {
	s.mu.Lock()
	s.routes[route] = &listener{status: Status{Error: err.Error()}, pairs: map[string]time.Time{}}
	s.mu.Unlock()
	return err
}

// Stop は全部の経路の待ち受けを止める
func (s *Server) Stop() {
	s.StopRoute(RouteLAN)
	s.StopRoute(RoutePublic)
}

// StopRoute は経路の待ち受けを止め、その経路でつないでいる画面を切る
func (s *Server) StopRoute(route string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	l := s.routes[route]
	delete(s.routes, route)
	var drop []*client
	for c := range s.clients {
		if c.route == route {
			drop = append(drop, c)
			delete(s.clients, c)
		}
	}
	s.mu.Unlock()
	for _, c := range drop {
		c.close()
	}
	if l != nil {
		if l.stop != nil {
			l.stop()
		}
		if l.srv != nil {
			l.srv.Close()
		}
	}
}

// Status は経路の待ち受けの様子。route を省くと LAN 内
func (s *Server) Status(route ...string) Status {
	r := RouteLAN
	if len(route) > 0 {
		r = route[0]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if l := s.routes[r]; l != nil {
		return l.status
	}
	return Status{}
}

// pairLimit は同時に使えるペアリング用のトークンの数。超えたら古いものから消す
const pairLimit = 5

// NewPairURL は、経路 route の新しいペアリング用のトークンを入れた URL を返す。route を省くと LAN 内
func (s *Server) NewPairURL(route ...string) (string, time.Time, error) {
	r := RouteLAN
	if len(route) > 0 {
		r = route[0]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.routes[r]
	if l == nil || !l.status.Running {
		return "", time.Time{}, errors.New("スマホの待ち受けが止まっています")
	}
	if l.status.URL == "" {
		return "", time.Time{}, errors.New("まだ証明書を取っています")
	}
	now := time.Now()
	for t, exp := range l.pairs {
		if now.After(exp) {
			delete(l.pairs, t)
		}
	}
	for len(l.pairs) >= pairLimit {
		var oldest string
		for t, exp := range l.pairs {
			if oldest == "" || exp.Before(l.pairs[oldest]) {
				oldest = t
			}
		}
		delete(l.pairs, oldest)
	}
	t := rand.Text()
	l.pairs[t] = now.Add(PairTTL)
	return l.status.URL + "/pair?t=" + t, l.pairs[t], nil
}

// takePair は、経路 route のトークンが合っていて期限内なら使用済みにして true を返す
func (s *Server) takePair(route, t string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.routes[route]
	if l == nil {
		return false
	}
	for p, exp := range l.pairs {
		if subtle.ConstantTimeCompare([]byte(t), []byte(p)) == 1 {
			delete(l.pairs, p)
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

// Record は、スマホからの操作を履歴に残す
func (s *Server) Record(dev Device, what string) {
	s.devs.log(Login{Time: time.Now(), Kind: "op", Device: dev.Name, IP: dev.LastIP, Route: dev.Route, What: what})
	s.changed()
}

// Note は、スマホ以外 (AI エージェントなど) の操作をスマホの履歴に残す
func (s *Server) Note(kind, route, what string) {
	s.devs.log(Login{Time: time.Now(), Kind: kind, Device: "AI エージェント", Route: route, What: what})
	s.changed()
}

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

// device は cookie の資格情報から端末を探す。ほかの経路で登録した端末は入れない
func (s *Server) device(route string, r *http.Request) (Device, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return Device{}, false
	}
	dev, ok := s.devs.check(c.Value)
	return dev, ok && dev.Route == route
}

func (s *Server) handler(route string) http.Handler {
	mux := http.NewServeMux()
	assets := http.FileServerFS(s.opts.Assets)

	mux.HandleFunc("GET /pair", func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !s.takePair(route, r.URL.Query().Get("t")) {
			s.devs.log(Login{Time: time.Now(), Kind: "pair-failed", IP: ip, Route: route})
			s.changed()
			page(w, r, http.StatusGone, pageExpired)
			return
		}
		cred, dev, err := s.devs.add(deviceName(r.UserAgent()), route, ip)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// ブラウザが持てる最長の 400 日にする。なくしたときは PC アプリから無効にする
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: cred, Path: "/", MaxAge: 400 * 24 * 3600, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		s.devs.log(Login{Time: time.Now(), Kind: "pair", Device: dev.Name, IP: ip, Route: route})
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
		dev, ok := s.device(route, r)
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		s.serveWS(route, dev, ws, clientIP(r))
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.device(route, r); !ok {
			if r.URL.Path == "/" {
				s.devs.log(Login{Time: time.Now(), Kind: "denied", IP: clientIP(r), Route: route})
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

func (s *Server) serveWS(route string, dev Device, ws *websocket.Conn, ip string) {
	ws.SetReadLimit(32 << 20)
	c := &client{route: route, dev: dev, ws: ws, out: make(chan []byte, 4096), done: make(chan struct{})}
	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()
	s.devs.seen(dev.ID, ip)
	dev.LastIP = ip
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
