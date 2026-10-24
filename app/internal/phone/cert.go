package phone

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// certDays は自己署名の証明書の有効期間。iOS は 825 日を超えるものを受け付けない
const certDays = 800

// loadCert は dir に置いた自己署名の証明書を読む。
// 無いとき、ip が SAN に入っていないとき、残りが30日を切ったときは作り直して置く。
// 作り直すとスマホでもう一度ブラウザの警告を受け入れることになるので、それ以外では作り直さない
func loadCert(dir string, ip net.IP) (tls.Certificate, error) {
	crtPath, keyPath := filepath.Join(dir, "lan.crt"), filepath.Join(dir, "lan.key")
	if c, err := tls.LoadX509KeyPair(crtPath, keyPath); err == nil && c.Leaf != nil {
		fresh := time.Until(c.Leaf.NotAfter) > 30*24*time.Hour
		covers := ip == nil || slices.ContainsFunc(c.Leaf.IPAddresses, ip.Equal)
		if fresh && covers {
			return c, nil
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	ips := []net.IP{net.IPv4(127, 0, 0, 1)}
	if ip != nil {
		ips = append(ips, ip)
	}
	// iOS は SAN と serverAuth が無いものを受け付けない
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "DRAG"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(certDays * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  ips,
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(crtPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return tls.Certificate{}, err
	}
	return tls.LoadX509KeyPair(crtPath, keyPath)
}

// lanIP は、この PC から外へ出るときに使う IPv4 アドレス。
// UDP の connect はパケットを送らずに経路だけ決めるので、既定の経路のアダプターが分かる
func lanIP() net.IP {
	if c, err := net.Dial("udp4", "192.0.2.1:9"); err == nil {
		defer c.Close()
		if a, ok := c.LocalAddr().(*net.UDPAddr); ok && !a.IP.IsLoopback() {
			return a.IP.To4()
		}
	}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() && n.IP.IsPrivate() {
			return n.IP.To4()
		}
	}
	return nil
}
