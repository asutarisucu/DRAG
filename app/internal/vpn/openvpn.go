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

	"github.com/asutarisucu/DRAG/app/internal/store"
	"github.com/asutarisucu/DRAG/app/internal/vpn/ovpn"
)

// ovpnMTU は VPN の中の IP のパケットの大きさ。OpenVPN の包みを足しても 1500 に収まるようにする
const ovpnMTU = 1400

func checkOpenVPN(v store.VPN) (Summary, error) {
	c, err := ovpn.Parse(v.Config)
	if err != nil {
		return Summary{}, err
	}
	r := c.Remotes[0]
	proto := "UDP"
	if r.TCP {
		proto = "TCP"
	}
	return Summary{Target: net.JoinHostPort(r.Host, r.Port) + " (" + proto + ")", Login: c.AuthUserPass && c.User == ""}, nil
}

type ovpnTunnel struct {
	c       *ovpn.Client
	tdev    tun.Device
	net     *netstack.Net
	sum     Summary
	hasDNS  bool
	closing sync.Once
}

func openOpenVPN(ctx context.Context, v store.VPN) (Tunnel, error) {
	cfg, err := ovpn.Parse(v.Config)
	if err != nil {
		return nil, err
	}
	c, err := ovpn.Dial(ctx, cfg, ovpn.Options{User: v.User, Pass: v.Password})
	if err != nil {
		return nil, err
	}
	p := c.Pushed()
	tdev, tnet, err := netstack.CreateNetTUN(p.Addrs, p.DNS, ovpnMTU)
	if err != nil {
		c.Close()
		return nil, err
	}
	sum, _ := checkOpenVPN(v)
	sum.Address = p.Addrs[0].String()
	var routes []string
	for _, r := range p.Routes {
		routes = append(routes, r.String())
	}
	sum.Routes = strings.Join(routes, ", ")
	t := &ovpnTunnel{c: c, tdev: tdev, net: tnet, sum: sum, hasDNS: len(p.DNS) > 0}
	go t.pumpOut()
	go t.pumpIn()
	return t, nil
}

// pumpOut は、アプリの中のネットワークスタックから出る IP のパケットを VPN に送る
func (t *ovpnTunnel) pumpOut() {
	bufs, sizes := [][]byte{make([]byte, 65535)}, []int{0}
	for {
		n, err := t.tdev.Read(bufs, sizes, 0)
		if err != nil {
			t.Close()
			return
		}
		if n == 1 {
			if t.c.WriteIP(append([]byte{}, bufs[0][:sizes[0]]...)) != nil {
				select {
				case <-t.c.Done():
					t.Close()
					return
				default:
				}
			}
		}
	}
}

// pumpIn は、VPN から届いた IP のパケットをネットワークスタックに渡す
func (t *ovpnTunnel) pumpIn() {
	for {
		p, err := t.c.ReadIP()
		if err != nil {
			t.Close()
			return
		}
		t.tdev.Write([][]byte{p}, 0)
	}
}

func (t *ovpnTunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
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
	case <-t.c.Done():
		return nil, fmt.Errorf("VPN が切れています: %w", t.c.Err())
	default:
	}
	return t.net.DialContextTCPAddrPort(ctx, netip.AddrPortFrom(ip, uint16(pn)))
}

func (t *ovpnTunnel) Summary() Summary { return t.sum }

func (t *ovpnTunnel) Done() <-chan struct{} { return t.c.Done() }

func (t *ovpnTunnel) Close() error {
	t.closing.Do(func() {
		t.c.Close()
		t.tdev.Close()
	})
	return nil
}
