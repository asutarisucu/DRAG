package vpn

import (
	"context"
	"crypto/x509"
	"errors"
	"strings"

	"github.com/asutarisucu/DRAG/app/internal/store"
	"github.com/asutarisucu/DRAG/app/internal/vpn/ikev2"
)

// ikev2CA は、サーバーの証明書を確かめる CA。空なら Windows が信頼する CA を使う
func ikev2CA(v store.VPN) (*x509.CertPool, error) {
	if strings.TrimSpace(v.CA) == "" {
		return nil, nil
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(v.CA)) {
		return nil, errors.New("CA の証明書を読めません")
	}
	return pool, nil
}

func checkIKEv2(v store.VPN) (Summary, error) {
	if strings.TrimSpace(v.Server) == "" {
		return Summary{}, errors.New("サーバーを入れてください")
	}
	if v.User == "" || v.Password == "" {
		return Summary{}, errors.New("ユーザー名とパスワードを入れてください")
	}
	if _, err := ikev2CA(v); err != nil {
		return Summary{}, err
	}
	return Summary{Target: strings.TrimSpace(v.Server)}, nil
}

func openIKEv2(ctx context.Context, v store.VPN) (Tunnel, error) {
	sum, err := checkIKEv2(v)
	if err != nil {
		return nil, err
	}
	ca, _ := ikev2CA(v)
	c, err := ikev2.Dial(ctx, ikev2.Config{Server: sum.Target, User: v.User, Pass: v.Password, CA: ca})
	if err != nil {
		return nil, err
	}
	p := c.Pushed()
	return newStackTunnel(c, p.Addrs, p.DNS, p.Routes, sum)
}
