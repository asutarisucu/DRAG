package vpn

import (
	"context"
	"net/netip"

	"github.com/asutarisucu/DRAG/app/internal/store"
	"github.com/asutarisucu/DRAG/app/internal/vpn/anyconnect"
)

// openAnyConnect は AnyConnect 互換 (Cisco ASA、ocserv) の VPN を張る
func openAnyConnect(ctx context.Context, v store.VPN) (Tunnel, error) {
	sum, err := checkServerLogin(v)
	if err != nil {
		return nil, err
	}
	ca, _ := serverCA(v)
	c, err := anyconnect.Dial(ctx, anyconnect.Config{Server: sum.Target, Group: v.Group, User: v.User, Pass: v.Password, CA: ca, MTU: stackMTU})
	if err != nil {
		return nil, err
	}
	p := c.Pushed()
	return newStackTunnel(c, []netip.Addr{p.Addr}, p.DNS, p.Routes, sum)
}
