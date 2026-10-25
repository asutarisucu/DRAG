package vpn

import (
	"context"
	"net"

	"github.com/asutarisucu/DRAG/app/internal/store"
	"github.com/asutarisucu/DRAG/app/internal/vpn/ovpn"
)

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
	sum, _ := checkOpenVPN(v)
	return newStackTunnel(c, p.Addrs, p.DNS, p.Routes, sum)
}
