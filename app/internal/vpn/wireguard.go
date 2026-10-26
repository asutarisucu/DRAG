package vpn

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/asutarisucu/DRAG/app/internal/store"
)

// wgConfig は WireGuard の .conf (wg-quick の形) から読んだ設定。
// PostUp や Table のように Windows の側を変える項目は使わない
type wgConfig struct {
	key   []byte
	addrs []netip.Addr
	dns   []netip.Addr
	mtu   int
	peers []wgPeer
}

type wgPeer struct {
	key, psk  []byte
	endpoint  string // host:port。名前のこともある
	allowed   []netip.Prefix
	keepalive int
}

func parseWireGuard(conf string) (*wgConfig, error) {
	c := &wgConfig{mtu: 1420}
	var peer *wgPeer
	section := ""
	sc := bufio.NewScanner(strings.NewReader(conf))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if section == "peer" {
				c.peers = append(c.peers, wgPeer{})
				peer = &c.peers[len(c.peers)-1]
			}
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%d 行目を読めません: %s", n, line)
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		var err error
		switch {
		case section == "interface" && k == "privatekey":
			c.key, err = wgKey(v)
		case section == "interface" && k == "address":
			for _, s := range list(v) {
				p, perr := parsePrefixOrAddr(s)
				if perr != nil {
					return nil, fmt.Errorf("%d 行目の Address を読めません: %s", n, s)
				}
				c.addrs = append(c.addrs, p.Addr())
			}
		case section == "interface" && k == "dns":
			// 検索ドメインも並ぶので、アドレスだけを使う
			for _, s := range list(v) {
				if ip, perr := netip.ParseAddr(s); perr == nil {
					c.dns = append(c.dns, ip)
				}
			}
		case section == "interface" && k == "mtu":
			// wireguard-go は最初の MTU を確かめずに使い、範囲の外だと暗号にするところで panic する
			if c.mtu, err = strconv.Atoi(v); err == nil && (c.mtu < 576 || c.mtu > device.MaxContentSize) {
				err = fmt.Errorf("MTU は 576 から %d までです", device.MaxContentSize)
			}
		case section == "peer" && k == "publickey":
			peer.key, err = wgKey(v)
		case section == "peer" && k == "presharedkey":
			peer.psk, err = wgKey(v)
		case section == "peer" && k == "endpoint":
			peer.endpoint = v
		case section == "peer" && k == "allowedips":
			for _, s := range list(v) {
				p, perr := parsePrefixOrAddr(s)
				if perr != nil {
					return nil, fmt.Errorf("%d 行目の AllowedIPs を読めません: %s", n, s)
				}
				peer.allowed = append(peer.allowed, p.Masked())
			}
		case section == "peer" && k == "persistentkeepalive":
			if v != "off" {
				if peer.keepalive, err = strconv.Atoi(v); err == nil && (peer.keepalive < 0 || peer.keepalive > 65535) {
					err = errors.New("PersistentKeepalive は 0 から 65535 までです")
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("%d 行目を読めません: %w", n, err)
		}
	}
	switch {
	case c.key == nil:
		return nil, errors.New("[Interface] の PrivateKey がありません")
	case len(c.addrs) == 0:
		return nil, errors.New("[Interface] の Address がありません")
	case len(c.peers) == 0:
		return nil, errors.New("[Peer] がありません")
	}
	for i, p := range c.peers {
		if p.key == nil {
			return nil, fmt.Errorf("%d 番目の [Peer] に PublicKey がありません", i+1)
		}
		if p.endpoint == "" {
			return nil, fmt.Errorf("%d 番目の [Peer] に Endpoint がありません", i+1)
		}
	}
	return c, nil
}

func wgKey(s string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != 32 {
		return nil, errors.New("鍵の形が違います")
	}
	return b, nil
}

func list(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parsePrefixOrAddr(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		return netip.ParsePrefix(s)
	}
	ip, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(ip, ip.BitLen()), nil
}

func (c *wgConfig) summary() Summary {
	var routes []string
	for _, p := range c.peers {
		for _, a := range p.allowed {
			routes = append(routes, a.String())
		}
	}
	return Summary{Target: c.peers[0].endpoint, Address: c.addrs[0].String(), Routes: strings.Join(routes, ", ")}
}

// uapi は wireguard-go に渡す設定。Endpoint の名前はここで PC の DNS で引く
func (c *wgConfig) uapi(ctx context.Context) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\nreplace_peers=true\n", hex.EncodeToString(c.key))
	for _, p := range c.peers {
		host, port, err := net.SplitHostPort(p.endpoint)
		if err != nil {
			return "", fmt.Errorf("Endpoint を読めません: %s", p.endpoint)
		}
		ip, err := resolve(ctx, host, nil)
		if err != nil {
			return "", fmt.Errorf("Endpoint の %s を引けません: %w", host, err)
		}
		pn, err := strconv.ParseUint(port, 10, 16)
		if err != nil {
			return "", fmt.Errorf("Endpoint のポートを読めません: %s", p.endpoint)
		}
		fmt.Fprintf(&b, "public_key=%s\n", hex.EncodeToString(p.key))
		if p.psk != nil {
			fmt.Fprintf(&b, "preshared_key=%s\n", hex.EncodeToString(p.psk))
		}
		fmt.Fprintf(&b, "endpoint=%s\n", netip.AddrPortFrom(ip.Unmap(), uint16(pn)))
		fmt.Fprintf(&b, "persistent_keepalive_interval=%d\nreplace_allowed_ips=true\n", p.keepalive)
		for _, a := range p.allowed {
			fmt.Fprintf(&b, "allowed_ip=%s\n", a)
		}
	}
	return b.String(), nil
}

type wgTunnel struct {
	cfg  *wgConfig
	dev  *device.Device
	net  *netstack.Net
	done chan struct{}
	once sync.Once
}

func openWireGuard(ctx context.Context, v store.VPN, target string) (Tunnel, error) {
	c, err := parseWireGuard(v.Config)
	if err != nil {
		return nil, err
	}
	uapi, err := c.uapi(ctx)
	if err != nil {
		return nil, err
	}
	tdev, tnet, err := netstack.CreateNetTUN(c.addrs, c.dns, c.mtu)
	if err != nil {
		return nil, err
	}
	dev := device.NewDevice(tdev, newDialBind(), &device.Logger{Verbosef: device.DiscardLogf, Errorf: device.DiscardLogf})
	if err := dev.IpcSet(uapi); err != nil {
		dev.Close()
		return nil, err
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, err
	}
	t := &wgTunnel{cfg: c, dev: dev, net: tnet, done: make(chan struct{})}
	if err := t.handshake(ctx, target); err != nil {
		dev.Close()
		return nil, err
	}
	return t, nil
}

// handshake は、VPN の中へ UDP を1つ送って鍵の交換を始めさせ、終わるまで待つ。
// WireGuard は送るものが無いと相手と話さないので、こうしないと張れたかがわからない
func (t *wgTunnel) handshake(ctx context.Context, target string) error {
	probe, ok := t.probeAddr(target)
	if ok {
		if c, err := t.net.DialUDPAddrPort(netip.AddrPort{}, netip.AddrPortFrom(probe, 9)); err == nil {
			c.Write([]byte{0})
			c.Close()
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		st, err := t.dev.IpcGet()
		if err != nil {
			return err
		}
		for _, line := range strings.Split(st, "\n") {
			if v, ok := strings.CutPrefix(line, "last_handshake_time_sec="); ok && v != "0" {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return errors.New("VPN の相手から返事がありません。Endpoint と鍵を確かめてください")
		case <-tick.C:
		}
	}
}

// probeAddr は、鍵の交換を始めさせるために送る先。つなぐサーバーが通す範囲にあればそこへ、なければ範囲の最初のアドレスへ送る
func (t *wgTunnel) probeAddr(target string) (netip.Addr, bool) {
	if ip, err := netip.ParseAddr(target); err == nil && t.routed(ip) {
		return ip, true
	}
	for _, p := range t.cfg.peers {
		for _, a := range p.allowed {
			return a.Addr().Next(), true
		}
	}
	return netip.Addr{}, false
}

func (t *wgTunnel) routed(ip netip.Addr) bool {
	for _, p := range t.cfg.peers {
		for _, a := range p.allowed {
			if a.Contains(ip) {
				return true
			}
		}
	}
	return false
}

func (t *wgTunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	var lookup func(context.Context, string) ([]string, error)
	if len(t.cfg.dns) > 0 {
		lookup = t.net.LookupContextHost
	}
	ip, err := resolve(ctx, host, lookup)
	if err != nil {
		return nil, err
	}
	if !t.routed(ip) {
		return nil, fmt.Errorf("%s は VPN に通す範囲 (AllowedIPs) の外です", ip)
	}
	pn, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, err
	}
	return t.net.DialContextTCPAddrPort(ctx, netip.AddrPortFrom(ip, uint16(pn)))
}

func (t *wgTunnel) Summary() Summary { return t.cfg.summary() }

// Done は Close まで閉じない。WireGuard は相手が黙っても、送るものがあれば鍵の交換をやり直す
func (t *wgTunnel) Done() <-chan struct{} { return t.done }

func (t *wgTunnel) Close() error {
	t.once.Do(func() {
		t.dev.Close()
		close(t.done)
	})
	return nil
}
