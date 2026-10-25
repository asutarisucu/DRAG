package vpn

import (
	"context"
	"net/netip"

	"github.com/asutarisucu/DRAG/app/internal/store"
	"github.com/asutarisucu/DRAG/app/internal/vpn/l2tp"
)

// openL2TP は L2TP/IPsec (事前共有キー) の VPN を張る
func openL2TP(ctx context.Context, v store.VPN) (Tunnel, error) {
	sum, err := Check(v)
	if err != nil {
		return nil, err
	}
	c, err := l2tp.Dial(ctx, l2tp.Config{Server: sum.Target, PSK: v.PSK, User: v.User, Pass: v.Password, Logf: Logf})
	if err != nil {
		return nil, err
	}
	p := c.Pushed()
	return newStackTunnel(c, []netip.Addr{p.Addr}, p.DNS, nil, sum)
}
