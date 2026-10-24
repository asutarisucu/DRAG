package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/acme"
)

// ipCert は Let's Encrypt から IP アドレスの証明書を TLS-ALPN-01 で取り、待ち受けに出す。
// 検証の接続は、ルーターが 443 番をこの待ち受けに転送してくることを前提にする
type ipCert struct {
	client *acme.Client
	ip     string

	mu   sync.Mutex
	chal *tls.Certificate // 検証の接続に出す証明書。検証の間だけ持つ

	cur atomic.Pointer[tls.Certificate]
}

// loadKey は ACME のアカウントの鍵を読む。無ければ作って書き出す
func loadKey(path string) (*ecdsa.PrivateKey, error) {
	if b, err := os.ReadFile(path); err == nil {
		blk, _ := pem.Decode(b)
		if blk == nil {
			return nil, fmt.Errorf("%s を読めない", path)
		}
		return x509.ParseECPrivateKey(blk.Bytes)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	return key, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600)
}

func newIPCert(ctx context.Context, dirURL, keyPath, ip string) (*ipCert, error) {
	key, err := loadKey(keyPath)
	if err != nil {
		return nil, err
	}
	c := &acme.Client{Key: key, DirectoryURL: dirURL}
	if _, err := c.Register(ctx, &acme.Account{}, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return nil, err
	}
	return &ipCert{client: c, ip: ip}, nil
}

// getCertificate は tls.Config.GetCertificate に渡す。
// IP アドレスには SNI が付かないので、名前ではなく ALPN で検証の接続かどうかを見分ける
func (m *ipCert) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if slices.Contains(hello.SupportedProtos, acme.ALPNProto) {
		m.mu.Lock()
		c := m.chal
		m.mu.Unlock()
		log.Printf("検証の接続 %s SNI=%q", hello.Conn.RemoteAddr(), hello.ServerName)
		if c == nil {
			return nil, errors.New("応える検証がない")
		}
		return c, nil
	}
	if c := m.cur.Load(); c != nil {
		return c, nil
	}
	return nil, errors.New("まだ証明書がない")
}

// obtain は証明書を1枚取り、待ち受けに出す証明書を差し替える
func (m *ipCert) obtain(ctx context.Context) error {
	t := time.Now()
	o, err := m.client.AuthorizeOrder(ctx, acme.IPIDs(m.ip), acme.WithOrderIssuanceProfile("shortlived"))
	if err != nil {
		return err
	}
	for _, u := range o.AuthzURLs {
		a, err := m.client.GetAuthorization(ctx, u)
		if err != nil {
			return err
		}
		if a.Status == acme.StatusValid {
			continue
		}
		var chal *acme.Challenge
		for _, c := range a.Challenges {
			if c.Type == "tls-alpn-01" {
				chal = c
			}
		}
		if chal == nil {
			return errors.New("tls-alpn-01 の検証がない")
		}
		cert, err := m.client.TLSALPN01ChallengeCert(chal.Token, m.ip)
		if err != nil {
			return err
		}
		m.mu.Lock()
		m.chal = &cert
		m.mu.Unlock()
		if _, err := m.client.Accept(ctx, chal); err != nil {
			return err
		}
		_, err = m.client.WaitAuthorization(ctx, u)
		m.mu.Lock()
		m.chal = nil
		m.mu.Unlock()
		if err != nil {
			return err
		}
	}
	if o, err = m.client.WaitOrder(ctx, o.URI); err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{IPAddresses: []net.IP{net.ParseIP(m.ip)}}, key)
	if err != nil {
		return err
	}
	der, _, err := m.client.CreateOrderCert(ctx, o.FinalizeURL, csr, true)
	if err != nil {
		return err
	}
	leaf, err := x509.ParseCertificate(der[0])
	if err != nil {
		return err
	}
	m.cur.Store(&tls.Certificate{Certificate: der, PrivateKey: key, Leaf: leaf})
	log.Printf("証明書を取った %v  serial=%X  %s 〜 %s (%v)  発行者=%s  SAN=%v",
		time.Since(t).Round(time.Millisecond), leaf.SerialNumber, leaf.NotBefore.Local().Format(time.DateTime), leaf.NotAfter.Local().Format(time.DateTime),
		leaf.NotAfter.Sub(leaf.NotBefore), leaf.Issuer.CommonName, leaf.IPAddresses)
	return nil
}
