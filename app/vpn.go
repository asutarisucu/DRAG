package main

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/asutarisucu/DRAG/app/internal/store"
	"github.com/asutarisucu/DRAG/app/internal/vpn"
	"github.com/asutarisucu/DRAG/app/internal/vpn/tsvpn"
)

// サーバーへの接続だけを通す VPN。保存した VPN は vpns.dat に置き、張った VPN は同じ VPN のサーバーで使い回す

// tunnelIdle は、どのサーバーも使わなくなった VPN を閉じるまでの時間。
// 接続の画面で VPN を張ってから SSH でつなぐまでのあいだも、この時間だけ残す
const tunnelIdle = 30 * time.Second

type tunnels struct {
	mu sync.Mutex
	m  map[string]*tunnel
}

// tunnel は張っている VPN 1つ。refs はこの VPN でつないでいるサーバーの数
type tunnel struct {
	vpn.Tunnel
	refs int
	idle *time.Timer
}

// VPNInput は、接続の画面で足す VPN
type VPNInput struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Config   string `json:"config"` // WireGuard の .conf か OpenVPN の .ovpn の中身
	Server   string `json:"server"` // IKEv2 などのつなぎ先
	CA       string `json:"ca"`     // サーバーの証明書を確かめる CA (PEM)
	Group    string `json:"group"`  // AnyConnect 互換のグループ
	PSK      string `json:"psk"`    // L2TP/IPsec の事前共有キー
	User     string `json:"user"`
	Password string `json:"password"`
}

// VPNInfo は画面に出す VPN。鍵とパスワードは渡さない
type VPNInfo struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Type    string      `json:"type"`
	Summary vpn.Summary `json:"summary"`
	Servers []string    `json:"servers"` // この VPN を使う保存済みのサーバーの名前
	InUse   bool        `json:"inUse"`   // この VPN でつないでいるサーバーがある
}

func (in VPNInput) store() store.VPN {
	return store.VPN{Name: strings.TrimSpace(in.Name), Type: in.Type, Config: in.Config, Server: strings.TrimSpace(in.Server), CA: in.CA, Group: strings.TrimSpace(in.Group), PSK: in.PSK, User: strings.TrimSpace(in.User), Password: in.Password}
}

// VPNs は保存した VPN を返す
func (a *App) VPNs() ([]VPNInfo, error) {
	list, err := a.vpns.List()
	if err != nil {
		return nil, err
	}
	saved, _ := a.saved.List()
	a.mu.Lock()
	using := map[string]bool{}
	for _, c := range a.servers {
		using[c.req.VPN] = true
	}
	a.mu.Unlock()
	out := []VPNInfo{}
	for _, v := range list {
		sum, _ := vpn.Check(v)
		info := VPNInfo{ID: v.ID, Name: v.Name, Type: v.Type, Summary: sum, Servers: []string{}, InUse: using[v.ID]}
		for _, s := range saved {
			if s.VPN == v.ID {
				info.Servers = append(info.Servers, s.Name)
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// CheckVPN は、足す前の VPN の設定を読み、要点を返す。読めなければ何が足りないかを返す
func (a *App) CheckVPN(in VPNInput) (vpn.Summary, error) {
	v := in.store()
	a.tsLogins.saved(&v)
	return vpn.Check(v)
}

// AddVPN は VPN を保存する。名前が空なら、つなぎ先のホスト名を名前にする
func (a *App) AddVPN(in VPNInput) (VPNInfo, error) {
	v := in.store()
	login := a.tsLogins.saved(&v)
	sum, err := vpn.Check(v)
	if err != nil {
		return VPNInfo{}, err
	}
	if v.Name == "" {
		v.Name = sum.Target
		if i := strings.LastIndexByte(v.Name, ':'); i > 0 {
			v.Name = v.Name[:i]
		}
	}
	v, err = a.vpns.Add(v)
	if err != nil {
		return VPNInfo{}, err
	}
	a.tsLogins.drop(login)
	return VPNInfo{ID: v.ID, Name: v.Name, Type: v.Type, Summary: sum, Servers: []string{}}, nil
}

// VPNDetail は、編集の画面に出す保存した VPN。パスワードと事前共有キーは渡さない
func (a *App) VPNDetail(id string) (VPNInput, error) {
	v, err := a.vpns.Get(id)
	if err != nil {
		return VPNInput{}, err
	}
	in := VPNInput{Name: v.Name, Type: v.Type, Config: v.Config, Server: v.Server, CA: v.CA, Group: v.Group, User: v.User}
	if v.Type == "ts" {
		in.Config = "" // 端末の鍵は渡さない
	}
	return in, nil
}

// UpdateVPN は保存した VPN を書き換える。種類は変えない。パスワードと事前共有キーは、空なら今のまま。
// Tailscale はログインを変えず、名前だけを書き換える
func (a *App) UpdateVPN(id string, in VPNInput) (VPNInfo, error) {
	if err := a.vpnIdle(id); err != nil {
		return VPNInfo{}, err
	}
	old, err := a.vpns.Get(id)
	if err != nil {
		return VPNInfo{}, err
	}
	v := in.store()
	v.ID, v.Type = id, old.Type
	if v.Password == "" {
		v.Password = old.Password
	}
	if v.PSK == "" {
		v.PSK = old.PSK
	}
	if v.Type == "ts" {
		v.Config = old.Config
	}
	sum, err := vpn.Check(v)
	if err != nil {
		return VPNInfo{}, err
	}
	if v.Name == "" {
		v.Name = old.Name
	}
	if err := a.vpns.Update(v); err != nil {
		return VPNInfo{}, err
	}
	a.forgetTunnel(id)
	return VPNInfo{ID: v.ID, Name: v.Name, Type: v.Type, Summary: sum, Servers: []string{}}, nil
}

// vpnIdle は、この VPN でつないでいるサーバーがなければ nil を返す
func (a *App) vpnIdle(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, c := range a.servers {
		if c.req.VPN == id {
			return errors.New("この VPN でつないでいるサーバーがあります")
		}
	}
	return nil
}

// forgetTunnel は、張ったままにしている VPN を閉じる。次に使うときは保存した設定で張り直す
func (a *App) forgetTunnel(id string) {
	a.tunnels.mu.Lock()
	defer a.tunnels.mu.Unlock()
	if t := a.tunnels.m[id]; t != nil {
		t.idle.Stop()
		t.Close()
		delete(a.tunnels.m, id)
	}
}

// DeleteVPN は保存した VPN を消す。この VPN を使う保存済みのサーバーは、VPN なしに戻す
func (a *App) DeleteVPN(id string) error {
	if err := a.vpnIdle(id); err != nil {
		return err
	}
	if err := a.vpns.Remove(id); err != nil {
		return err
	}
	a.forgetTunnel(id)
	saved, err := a.saved.List()
	if err != nil {
		return err
	}
	for _, s := range saved {
		if s.VPN == id {
			s.VPN = ""
			if _, err := a.saved.Put(s); err != nil {
				return err
			}
		}
	}
	return nil
}

// ConnectVPN は VPN を張る。同じ VPN を張っていればそれを使う。
// host はつなぐサーバーで、張れたかを確かめるのに使う
func (a *App) ConnectVPN(id, host string) (vpn.Summary, error) {
	t, err := a.useTunnel(id, host)
	if err != nil {
		return vpn.Summary{}, err
	}
	return t.Summary(), nil
}

// useTunnel は張っている VPN を返す。無ければ張る。どのサーバーも使っていなければ、閉じるまでの時間を延ばす
func (a *App) useTunnel(id, host string) (vpn.Tunnel, error) {
	a.tunnels.mu.Lock()
	refs := 0
	if t := a.tunnels.m[id]; t != nil {
		select {
		case <-t.Done():
			// VPN が切れていたら張り直す。使っているサーバーの数は引き継ぐ
			refs = t.refs
			t.idle.Stop()
			delete(a.tunnels.m, id)
		default:
			if t.refs == 0 {
				t.idle.Reset(tunnelIdle)
			}
			a.tunnels.mu.Unlock()
			return t, nil
		}
	}
	a.tunnels.mu.Unlock()

	v, err := a.vpns.Get(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	opened, err := vpn.Open(ctx, v, host)
	if err != nil {
		return nil, err
	}
	a.tunnels.mu.Lock()
	defer a.tunnels.mu.Unlock()
	// 張っているあいだに、ほかの端末が同じ VPN を張っていたら、そちらを使う
	if t := a.tunnels.m[id]; t != nil {
		opened.Close()
		return t, nil
	}
	t := &tunnel{Tunnel: opened, refs: refs}
	t.idle = time.AfterFunc(tunnelIdle, func() { a.closeIdleTunnel(id, t) })
	if refs > 0 {
		t.idle.Stop()
	}
	a.tunnels.m[id] = t
	return t, nil
}

func (a *App) closeIdleTunnel(id string, t *tunnel) {
	a.tunnels.mu.Lock()
	defer a.tunnels.mu.Unlock()
	if a.tunnels.m[id] != t || t.refs > 0 {
		return
	}
	t.Close()
	delete(a.tunnels.m, id)
}

// holdTunnel と dropTunnel は、VPN でつないでいるサーバーの数を数える。0 になったら少し待って閉じる
func (a *App) holdTunnel(id string) {
	a.tunnels.mu.Lock()
	defer a.tunnels.mu.Unlock()
	if t := a.tunnels.m[id]; t != nil {
		t.refs++
		t.idle.Stop()
	}
}

func (a *App) dropTunnel(id string) {
	a.tunnels.mu.Lock()
	defer a.tunnels.mu.Unlock()
	if t := a.tunnels.m[id]; t != nil && t.refs > 0 {
		t.refs--
		if t.refs == 0 {
			t.idle.Reset(tunnelIdle)
		}
	}
}

func (a *App) closeTunnels() {
	a.tunnels.mu.Lock()
	defer a.tunnels.mu.Unlock()
	for id, t := range a.tunnels.m {
		t.idle.Stop()
		if err := t.Close(); err != nil {
			runtime.LogWarningf(a.ctx, "VPN を閉じられません: %v", err)
		}
		delete(a.tunnels.m, id)
	}
}

// tsLogins は、接続の画面で始めた Tailscale のログイン。VPN を足すか 10 分たつまで持つ
type tsLogins struct {
	mu sync.Mutex
	m  map[string]*tsvpn.Node
}

// TSLogin は始めたログイン。URL はブラウザで開いたページ
type TSLogin struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// TSInfo は加わった tailnet と、そこでのこの PC の名前
type TSInfo struct {
	Tailnet string `json:"tailnet"`
	Node    string `json:"node"`
}

// TailscaleLogin は、DRAG を新しい端末として tailnet に加えるログインを始め、そのページをブラウザで開く
func (a *App) TailscaleLogin() (TSLogin, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	n, url, err := tsvpn.Login(ctx, "")
	if err != nil {
		return TSLogin{}, err
	}
	id := rand.Text()
	a.tsLogins.mu.Lock()
	a.tsLogins.m[id] = n
	a.tsLogins.mu.Unlock()
	time.AfterFunc(10*time.Minute, func() { a.tsLogins.drop(id) })
	if url != "" {
		runtime.BrowserOpenURL(a.ctx, url)
	}
	return TSLogin{ID: id, URL: url}, nil
}

// TailscaleWait は、ブラウザでのログインが済むまで待つ
func (a *App) TailscaleWait(id string) (TSInfo, error) {
	a.tsLogins.mu.Lock()
	n := a.tsLogins.m[id]
	a.tsLogins.mu.Unlock()
	if n == nil {
		return TSInfo{}, errors.New("Tailscale のログインが見つかりません。もう一度ログインしてください")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := n.Wait(ctx); err != nil {
		return TSInfo{}, err
	}
	return TSInfo{Tailnet: n.Tailnet(), Node: n.Name()}, nil
}

// saved は、Tailscale の VPN の Config に入っているログインの番号を、保存する中身に置き換える。番号を返す
func (l *tsLogins) saved(v *store.VPN) string {
	if v.Type != "ts" {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	id := v.Config
	n := l.m[id]
	if n == nil || n.Tailnet() == "" {
		v.Config = ""
		return ""
	}
	v.Config = n.Saved()
	return id
}

func (l *tsLogins) drop(id string) {
	l.mu.Lock()
	n := l.m[id]
	delete(l.m, id)
	l.mu.Unlock()
	if n != nil {
		n.Close()
	}
}
