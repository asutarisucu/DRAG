package vpn

import (
	"crypto/x509"
	"errors"
	"strings"

	"github.com/asutarisucu/DRAG/app/internal/store"
)

// serverCA は、サーバーの証明書を確かめる CA。空なら Windows が信頼する CA を使う
func serverCA(v store.VPN) (*x509.CertPool, error) {
	if strings.TrimSpace(v.CA) == "" {
		return nil, nil
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(v.CA)) {
		return nil, errors.New("CA の証明書を読めません")
	}
	return pool, nil
}

// checkServerLogin は、サーバーとユーザー名とパスワードで入る種類 (IKEv2、SSTP) の設定を確かめる
func checkServerLogin(v store.VPN) (Summary, error) {
	if strings.TrimSpace(v.Server) == "" {
		return Summary{}, errors.New("サーバーを入れてください")
	}
	if v.User == "" || v.Password == "" {
		return Summary{}, errors.New("ユーザー名とパスワードを入れてください")
	}
	if _, err := serverCA(v); err != nil {
		return Summary{}, err
	}
	return Summary{Target: strings.TrimSpace(v.Server)}, nil
}
