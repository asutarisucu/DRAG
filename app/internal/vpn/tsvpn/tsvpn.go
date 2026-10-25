// Package tsvpn は、Tailscale の公式のライブラリ (tailscale.com/tsnet v1.104.2) で、DRAG を tailnet の端末の1つとして加える。
// Windows に Tailscale のアダプターを足さず、アプリの中のネットワークスタックで tailnet の中へつなぐ
package tsvpn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"

	"tailscale.com/ipn"
	"tailscale.com/ipn/ipnstate"
	"tailscale.com/logtail"
	"tailscale.com/tsnet"
)

func init() {
	// tsnet は既定で Tailscale のサーバーへログを送る。DRAG では送らない
	logtail.Disable()
}

// memStore は tsnet の状態 (端末の鍵と設定)。ファイルには書かず、呼ぶ側が VPN の設定ごと DPAPI で守って保存する
type memStore struct {
	mu sync.Mutex
	m  map[ipn.StateKey][]byte
}

func (s *memStore) ReadState(id ipn.StateKey) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[id]; ok {
		return v, nil
	}
	return nil, ipn.ErrStateNotExist
}

func (s *memStore) WriteState(id ipn.StateKey, bs []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bs == nil {
		delete(s.m, id)
	} else {
		s.m[id] = append([]byte{}, bs...)
	}
	return nil
}

// Saved は保存する中身。Control が空なら Tailscale のログインサーバー
type Saved struct {
	Tailnet string                  `json:"tailnet"`
	Node    string                  `json:"node"`
	Control string                  `json:"control,omitempty"`
	State   map[ipn.StateKey][]byte `json:"state"`
}

// Parse は保存した中身を読む
func Parse(config string) (Saved, error) {
	var s Saved
	if err := json.Unmarshal([]byte(config), &s); err != nil || len(s.State) == 0 {
		return Saved{}, errors.New("Tailscale のログインの情報を読めません")
	}
	return s, nil
}

// Node は tailnet に加わった DRAG の端末1つ
type Node struct {
	srv     *tsnet.Server
	st      *memStore
	dir     string
	control string
	addr    netip.Addr
	tailnet string
	name    string

	done chan struct{}
	once sync.Once
	err  error
}

// hostname は tailnet の中での名前。PC の名前に drag- を付ける
func hostname() string {
	h, _ := os.Hostname()
	h = strings.ToLower(h)
	if h == "" {
		return "drag"
	}
	return "drag-" + h
}

func start(state map[ipn.StateKey][]byte, control, authKey string) (*Node, error) {
	dir, err := os.MkdirTemp("", "drag-tailscale-")
	if err != nil {
		return nil, err
	}
	st := &memStore{m: map[ipn.StateKey][]byte{}}
	for k, v := range state {
		st.m[k] = v
	}
	discard := func(string, ...any) {}
	n := &Node{st: st, dir: dir, control: control, done: make(chan struct{})}
	n.srv = &tsnet.Server{Dir: dir, Store: st, Hostname: hostname(), ControlURL: control, AuthKey: authKey, UserLogf: discard, Logf: discard}
	if err := n.srv.Start(); err != nil {
		n.Close()
		return nil, fmt.Errorf("Tailscale を始められません: %w", err)
	}
	return n, nil
}

// Login は新しい端末として tailnet へのログインを始め、ブラウザで開く URL を返す
func Login(ctx context.Context, control string) (*Node, string, error) {
	n, err := start(nil, control, "")
	if err != nil {
		return nil, "", err
	}
	url, err := n.watch(ctx, true)
	if err != nil {
		n.Close()
		return nil, "", err
	}
	return n, url, nil
}

// watch は、ログインの URL が出るか、tailnet に加わるまで待つ。
// forURL が false なら、ログインを求められたときに失敗にする
func (n *Node) watch(ctx context.Context, forURL bool) (string, error) {
	lc, err := n.srv.LocalClient()
	if err != nil {
		return "", err
	}
	w, err := lc.WatchIPNBus(ctx, ipn.NotifyInitialState)
	if err != nil {
		return "", err
	}
	defer w.Close()
	for {
		m, err := w.Next()
		if err != nil {
			if ctx.Err() != nil {
				return "", errors.New("Tailscale から返事がありません")
			}
			return "", err
		}
		if m.ErrMessage != nil {
			return "", fmt.Errorf("Tailscale: %s", *m.ErrMessage)
		}
		if m.BrowseToURL != nil && *m.BrowseToURL != "" {
			if forURL {
				return *m.BrowseToURL, nil
			}
			return "", errors.New("Tailscale のログインが切れています。この VPN を消して、ログインし直してください")
		}
		if m.State != nil && *m.State == ipn.Running {
			st, err := lc.Status(ctx)
			if err != nil {
				return "", err
			}
			n.status(st)
			return "", nil
		}
	}
}

func (n *Node) status(st *ipnstate.Status) {
	for _, a := range st.TailscaleIPs {
		if a.Is4() {
			n.addr = a
		}
	}
	if st.CurrentTailnet != nil {
		n.tailnet = st.CurrentTailnet.Name
	}
	if st.Self != nil {
		n.name = st.Self.HostName
	}
}

// Wait はブラウザでのログインが済み、tailnet に加わるまで待つ
func (n *Node) Wait(ctx context.Context) error {
	st, err := n.srv.Up(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return errors.New("Tailscale のログインが済みませんでした")
		}
		return err
	}
	n.status(st)
	return nil
}

// Saved は、いまの状態を保存する形にする
func (n *Node) Saved() string {
	n.st.mu.Lock()
	defer n.st.mu.Unlock()
	b, _ := json.Marshal(Saved{Tailnet: n.tailnet, Node: n.name, Control: n.control, State: n.st.m})
	return string(b)
}

// Open は保存した状態で tailnet に加わる
func Open(ctx context.Context, s Saved) (*Node, error) {
	return open(ctx, s, "")
}

func open(ctx context.Context, s Saved, authKey string) (*Node, error) {
	n, err := start(s.State, s.Control, authKey)
	if err != nil {
		return nil, err
	}
	if _, err := n.watch(ctx, false); err != nil {
		n.Close()
		return nil, err
	}
	go n.follow()
	return n, nil
}

// follow は、tailnet から外れたら (ログインが切れたなど) Done を閉じる
func (n *Node) follow() {
	lc, err := n.srv.LocalClient()
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-n.done
		cancel()
	}()
	w, err := lc.WatchIPNBus(ctx, 0)
	if err != nil {
		return
	}
	defer w.Close()
	for {
		m, err := w.Next()
		if err != nil {
			return
		}
		if m.State != nil && *m.State != ipn.Running && *m.State != ipn.Starting {
			n.fail(fmt.Errorf("Tailscale から外れました (%s)", *m.State))
			return
		}
	}
}

// DialContext は tailnet の中へ TCP でつなぐ。名前は MagicDNS で引く
func (n *Node) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return n.srv.Dial(ctx, network, addr)
}

// Addr は tailnet の中でのこの端末のアドレス
func (n *Node) Addr() netip.Addr { return n.addr }

// Tailnet は加わった tailnet の名前
func (n *Node) Tailnet() string { return n.tailnet }

// Name は tailnet の中でのこの端末の名前
func (n *Node) Name() string { return n.name }

func (n *Node) Done() <-chan struct{} { return n.done }

func (n *Node) fail(err error) {
	n.once.Do(func() {
		n.err = err
		close(n.done)
	})
}

func (n *Node) Err() error {
	<-n.done
	return n.err
}

// Close は tsnet を止め、作業用のフォルダを消す
func (n *Node) Close() error {
	n.fail(net.ErrClosed)
	if n.srv != nil {
		n.srv.Close()
	}
	return os.RemoveAll(n.dir)
}
