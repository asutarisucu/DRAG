package vpn

import (
	"context"
	"net"

	"github.com/asutarisucu/DRAG/app/internal/store"
	"github.com/asutarisucu/DRAG/app/internal/vpn/tsvpn"
)

// tsTunnel は、DRAG を tailnet の端末の1つとして加えたもの
type tsTunnel struct {
	*tsvpn.Node
	sum Summary
}

func (t *tsTunnel) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return t.Node.DialContext(ctx, network, addr)
}

func (t *tsTunnel) Summary() Summary { return t.sum }

// openTailscale は、保存したログインで tailnet に加わる
func openTailscale(ctx context.Context, v store.VPN) (Tunnel, error) {
	s, err := tsvpn.Parse(v.Config)
	if err != nil {
		return nil, err
	}
	n, err := tsvpn.Open(ctx, s)
	if err != nil {
		return nil, err
	}
	return &tsTunnel{Node: n, sum: Summary{Target: n.Tailnet(), Address: n.Addr().String()}}, nil
}
