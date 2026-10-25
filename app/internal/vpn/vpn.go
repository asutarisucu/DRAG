// Package vpn は、DRAG の中だけで VPN を張る。
// Windows に仮想のアダプターや経路を足さず、アプリの中のネットワークスタック (gVisor) を通して
// サーバーへの接続だけを VPN に流す。PC のほかのソフトの通信は変わらない
package vpn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"

	"github.com/asutarisucu/DRAG/app/internal/store"
	"github.com/asutarisucu/DRAG/app/internal/vpn/tsvpn"
)

// Tunnel は張った VPN 1つ
type Tunnel interface {
	// DialContext は VPN を通して TCP でつなぐ。host が名前なら、VPN の DNS か PC の DNS で引く
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
	Summary() Summary
	// Done は VPN が切れると閉じる
	Done() <-chan struct{}
	Close() error
}

// Summary は設定から読んだ要点。画面で VPN の説明に使う
type Summary struct {
	Target  string `json:"target"`  // つなぎ先
	Address string `json:"address"` // VPN の中でのこの PC のアドレス。張るまでわからない種類では空
	Routes  string `json:"routes"`  // VPN に通す範囲。張るまでわからない種類では空
	Login   bool   `json:"login"`   // ユーザー名とパスワードが要る
}

// Check は、保存する前に設定を読めるか確かめ、要点を返す
func Check(v store.VPN) (Summary, error) {
	switch v.Type {
	case "wg":
		c, err := parseWireGuard(v.Config)
		if err != nil {
			return Summary{}, err
		}
		return c.summary(), nil
	case "ovpn":
		return checkOpenVPN(v)
	case "ikev2", "sstp", "ac":
		return checkServerLogin(v)
	case "l2tp":
		if v.PSK == "" {
			return Summary{}, errors.New("事前共有キーを入れてください")
		}
		return checkServerLogin(v)
	case "ts":
		s, err := tsvpn.Parse(v.Config)
		if err != nil {
			return Summary{}, errors.New("ブラウザで Tailscale にログインしてください")
		}
		return Summary{Target: s.Tailnet}, nil
	}
	return Summary{}, fmt.Errorf("この種類の VPN にはまだ対応していません: %s", v.Type)
}

// Logf は、VPN を張るときと通すときの様子を書く先。パスワードと鍵は書かない
var Logf = func(format string, args ...any) {}

// Open は VPN を張る。target はつなぐサーバーのアドレスで、張れたかを確かめるのに使う
func Open(ctx context.Context, v store.VPN, target string) (Tunnel, error) {
	Logf("VPN %s (%s) を張ります。つなぐサーバーは %s", v.Name, v.Type, target)
	t, err := open(ctx, v, target)
	if err != nil {
		Logf("VPN %s を張れません: %v", v.Name, err)
		return nil, err
	}
	s := t.Summary()
	Logf("VPN %s を張りました。つなぎ先 %s、この PC は %s、通す範囲 %s", v.Name, s.Target, s.Address, s.Routes)
	return t, nil
}

func open(ctx context.Context, v store.VPN, target string) (Tunnel, error) {
	switch v.Type {
	case "wg":
		return openWireGuard(ctx, v, target)
	case "ovpn":
		return openOpenVPN(ctx, v)
	case "ikev2":
		return openIKEv2(ctx, v)
	case "sstp":
		return openSSTP(ctx, v)
	case "ac":
		return openAnyConnect(ctx, v)
	case "l2tp":
		return openL2TP(ctx, v)
	case "ts":
		return openTailscale(ctx, v)
	}
	return nil, fmt.Errorf("この種類の VPN にはまだ対応していません: %s", v.Type)
}

// resolve は、VPN の DNS があればそれで、なければ PC の DNS で名前を引く
func resolve(ctx context.Context, host string, vpnLookup func(context.Context, string) ([]string, error)) (netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip, nil
	}
	lookup := net.DefaultResolver.LookupHost
	if vpnLookup != nil {
		lookup = vpnLookup
	}
	addrs, err := lookup(ctx, host)
	if err != nil {
		return netip.Addr{}, err
	}
	for _, a := range addrs {
		if ip, err := netip.ParseAddr(a); err == nil {
			return ip, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("%s のアドレスが見つかりません", host)
}
