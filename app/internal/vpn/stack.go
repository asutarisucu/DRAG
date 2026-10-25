package vpn

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"

	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// stackMTU は VPN の中の IP のパケットの大きさ。VPN の包みを足しても 1500 に収まるようにする
const stackMTU = 1400

// ipVPN は、IP のパケットを読み書きする VPN の接続 (OpenVPN、IKEv2 など)
type ipVPN interface {
	WriteIP(p []byte) error
	ReadIP() ([]byte, error)
	Done() <-chan struct{}
	Err() error
	Close() error
}

// stackTunnel は、ipVPN をアプリの中のネットワークスタックにつないだもの
type stackTunnel struct {
	vpn     ipVPN
	tdev    tun.Device
	net     *netstack.Net
	sum     Summary
	hasDNS  bool
	closing sync.Once
}

// newStackTunnel は、VPN からもらったアドレスと DNS でネットワークスタックを作り、パケットを渡し始める
func newStackTunnel(v ipVPN, addrs, dns []netip.Addr, routes []netip.Prefix, sum Summary) (*stackTunnel, error) {
	tdev, tnet, err := netstack.CreateNetTUN(addrs, dns, stackMTU)
	if err != nil {
		v.Close()
		return nil, err
	}
	sum.Address = addrs[0].String()
	var rs []string
	for _, r := range routes {
		rs = append(rs, r.String())
	}
	sum.Routes = strings.Join(rs, ", ")
	t := &stackTunnel{vpn: v, tdev: tdev, net: tnet, sum: sum, hasDNS: len(dns) > 0}
	go t.pumpIn()
	go t.pumpOut()
	return t, nil
}

// pumpOut は、ネットワークスタックから出る IP のパケットを VPN に送る
func (t *stackTunnel) pumpOut() {
	bufs, sizes := [][]byte{make([]byte, 65535)}, []int{0}
	for {
		n, err := t.tdev.Read(bufs, sizes, 0)
		if err != nil {
			t.Close()
			return
		}
		if n == 1 {
			t.vpn.WriteIP(append([]byte{}, bufs[0][:sizes[0]]...))
		}
	}
}

// pumpIn は、VPN から届いた IP のパケットをネットワークスタックに渡す
func (t *stackTunnel) pumpIn() {
	for {
		p, err := t.vpn.ReadIP()
		if err != nil {
			t.Close()
			return
		}
		t.tdev.Write([][]byte{p}, 0)
	}
}

func (t *stackTunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	var lookup func(context.Context, string) ([]string, error)
	if t.hasDNS {
		lookup = t.net.LookupContextHost
	}
	ip, err := resolve(ctx, host, lookup)
	if err != nil {
		return nil, err
	}
	pn, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, err
	}
	select {
	case <-t.vpn.Done():
		return nil, fmt.Errorf("VPN が切れています: %w", t.vpn.Err())
	default:
	}
	return t.net.DialContextTCPAddrPort(ctx, netip.AddrPortFrom(ip, uint16(pn)))
}

func (t *stackTunnel) Summary() Summary { return t.sum }

func (t *stackTunnel) Done() <-chan struct{} { return t.vpn.Done() }

func (t *stackTunnel) Close() error {
	t.closing.Do(func() {
		t.vpn.Close()
		t.tdev.Close()
	})
	return nil
}
