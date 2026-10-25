package vpn

import (
	"context"
	"net/netip"

	"github.com/asutarisucu/DRAG/app/internal/store"
	"github.com/asutarisucu/DRAG/app/internal/vpn/sstp"
)

func openSSTP(ctx context.Context, v store.VPN) (Tunnel, error) {
	sum, err := checkServerLogin(v)
	if err != nil {
		return nil, err
	}
	ca, _ := serverCA(v)
	c, err := sstp.Dial(ctx, sstp.Config{Server: sum.Target, User: v.User, Pass: v.Password, CA: ca})
	if err != nil {
		return nil, err
	}
	p := c.Pushed()
	return newStackTunnel(c, []netip.Addr{p.Addr}, p.DNS, nil, sum)
}
