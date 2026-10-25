package vpn

import (
	"context"

	"github.com/asutarisucu/DRAG/app/internal/store"
	"github.com/asutarisucu/DRAG/app/internal/vpn/ikev2"
)

func openIKEv2(ctx context.Context, v store.VPN) (Tunnel, error) {
	sum, err := checkServerLogin(v)
	if err != nil {
		return nil, err
	}
	ca, _ := serverCA(v)
	c, err := ikev2.Dial(ctx, ikev2.Config{Server: sum.Target, User: v.User, Pass: v.Password, CA: ca})
	if err != nil {
		return nil, err
	}
	p := c.Pushed()
	return newStackTunnel(c, p.Addrs, p.DNS, p.Routes, sum)
}
