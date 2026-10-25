package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/asutarisucu/DRAG/app/internal/store"
	"github.com/asutarisucu/DRAG/app/internal/vpn"
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
	return vpn.Check(in.store())
}

// AddVPN は VPN を保存する。名前が空なら、つなぎ先のホスト名を名前にする
func (a *App) AddVPN(in VPNInput) (VPNInfo, error) {
	v := in.store()
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
	return VPNInfo{ID: v.ID, Name: v.Name, Type: v.Type, Summary: sum, Servers: []string{}}, nil
}

// DeleteVPN は保存した VPN を消す。この VPN を使う保存済みのサーバーは、VPN なしに戻す
func (a *App) DeleteVPN(id string) error {
	a.mu.Lock()
	for _, c := range a.servers {
		if c.req.VPN == id {
			a.mu.Unlock()
			return errors.New("この VPN でつないでいるサーバーがあります")
		}
	}
	a.mu.Unlock()
	if err := a.vpns.Remove(id); err != nil {
		return err
	}
	a.tunnels.mu.Lock()
	if t := a.tunnels.m[id]; t != nil {
		t.idle.Stop()
		t.Close()
		delete(a.tunnels.m, id)
	}
	a.tunnels.mu.Unlock()
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
