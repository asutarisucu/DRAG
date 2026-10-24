package phone

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/acme"
)

// 直接公開。ルーターのポート転送で PC の待ち受けを外に出し、Let's Encrypt の証明書で HTTPS にする。
// 証明書は IP アドレスか自分のドメイン。IP アドレスは shortlived (160 時間) で、グローバル IP が変わったらその IP で取り直す。
// ドメインは DNS-01 で、DNS サービスの API で TXT レコードを置いて取る。どちらも有効期間の残りが半分を切ったら取り直す

// LetsEncrypt は Let's Encrypt の本番のディレクトリ
const LetsEncrypt = "https://acme-v02.api.letsencrypt.org/directory"

// pubCert は直接公開の証明書を取り、待ち受けに出す
type pubCert struct {
	client   *acme.Client
	port     int    // 外から入る待ち受けのポート。IP アドレスなら、443 は TLS-ALPN-01、ほかは httpAddr で HTTP-01 を使う
	httpAddr string // HTTP-01 の検証のときだけ待ち受けるアドレス。ふつうは :80
	// 自分のドメインの証明書を DNS-01 で取るときの DNS サービス。nil なら IP アドレスの証明書
	dns DNSProvider
	// TXT レコードを置いてから、Let's Encrypt に確かめてもらうまでに待つ時間
	dnsWait time.Duration

	mu   sync.Mutex
	chal *tls.Certificate  // TLS-ALPN-01 の検証の接続に出す証明書。検証の間だけ持つ
	http map[string]string // HTTP-01 のトークンと応え

	cur atomic.Pointer[tls.Certificate]
}

func loadAccountKey(path string) (*ecdsa.PrivateKey, error) {
	if b, err := os.ReadFile(path); err == nil {
		blk, _ := pem.Decode(b)
		if blk == nil {
			return nil, fmt.Errorf("%s を読めません", path)
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return key, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600)
}

func newPubCert(ctx context.Context, dirURL, keyPath string, port int, httpAddr string, hc *http.Client) (*pubCert, error) {
	key, err := loadAccountKey(keyPath)
	if err != nil {
		return nil, err
	}
	c := &acme.Client{Key: key, DirectoryURL: dirURL, HTTPClient: hc}
	if _, err := c.Register(ctx, &acme.Account{}, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return nil, err
	}
	return &pubCert{client: c, port: port, httpAddr: httpAddr, http: map[string]string{}}, nil
}

// getCertificate は tls.Config.GetCertificate に渡す。
// IP アドレスには SNI が付かないので、名前ではなく ALPN で検証の接続かどうかを見分ける
func (m *pubCert) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if slices.Contains(hello.SupportedProtos, acme.ALPNProto) {
		m.mu.Lock()
		c := m.chal
		m.mu.Unlock()
		if c == nil {
			return nil, errors.New("応える検証がありません")
		}
		return c, nil
	}
	if c := m.cur.Load(); c != nil {
		return c, nil
	}
	return nil, errors.New("まだ証明書がありません")
}

// obtain は証明書を1枚取り、待ち受けに出す証明書を差し替える。dns があれば id はドメイン、なければ IP アドレス
func (m *pubCert) obtain(ctx context.Context, id string) error {
	ids, opts, kind := acme.IPIDs(id), []acme.OrderOption{acme.WithOrderIssuanceProfile("shortlived")}, "tls-alpn-01"
	if m.dns != nil {
		ids, opts, kind = acme.DomainIDs(id), nil, "dns-01"
	} else if m.port != 443 {
		kind = "http-01"
	}
	o, err := m.client.AuthorizeOrder(ctx, ids, opts...)
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
			if c.Type == kind {
				chal = c
			}
		}
		if chal == nil {
			return fmt.Errorf("%s の検証がありません", kind)
		}
		stop, err := m.prepare(ctx, chal, id)
		if err != nil {
			return err
		}
		if _, err := m.client.Accept(ctx, chal); err != nil {
			stop()
			return err
		}
		_, err = m.client.WaitAuthorization(ctx, u)
		stop()
		if err != nil {
			switch kind {
			case "dns-01":
				return fmt.Errorf("Let's Encrypt が _acme-challenge.%s の TXT レコードを確かめられません: %w", id, err)
			case "tls-alpn-01":
				return fmt.Errorf("Let's Encrypt から %s:443 に届きません。ルーターのポート転送を確かめてください: %w", id, err)
			}
			return fmt.Errorf("Let's Encrypt から %s:80 に届きません。ルーターのポート転送を確かめてください: %w", id, err)
		}
	}
	// 待ったあとの注文には URL が入らない (Location が無い) ので、最初の注文の URL を付け直す。
	// CreateCertFromOrder は発行を待つのにこの URL を使う
	uri := o.URI
	if o, err = m.client.WaitOrder(ctx, uri); err != nil {
		return err
	}
	o.URI = uri
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	req := &x509.CertificateRequest{IPAddresses: []net.IP{net.ParseIP(id)}}
	if m.dns != nil {
		req = &x509.CertificateRequest{DNSNames: []string{id}}
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, req, key)
	if err != nil {
		return err
	}
	der, _, err := m.client.CreateCertFromOrder(ctx, o, csr, true)
	if err != nil {
		return err
	}
	leaf, err := x509.ParseCertificate(der[0])
	if err != nil {
		return err
	}
	m.cur.Store(&tls.Certificate{Certificate: der, PrivateKey: key, Leaf: leaf})
	return nil
}

// prepare は検証に応える用意をし、片付ける関数を返す。
// TLS-ALPN-01 は待ち受けの getCertificate で応え、HTTP-01 は検証の間だけ 80 番で待ち受ける。
// DNS-01 は DNS サービスに TXT レコードを置き、DNS に出るまで少し待つ
func (m *pubCert) prepare(ctx context.Context, chal *acme.Challenge, id string) (func(), error) {
	if chal.Type == "dns-01" {
		v, err := m.client.DNS01ChallengeRecord(chal.Token)
		if err != nil {
			return nil, err
		}
		remove, err := m.dns.SetTXT(ctx, "_acme-challenge."+id, v)
		if err != nil {
			return nil, err
		}
		select {
		case <-time.After(m.dnsWait):
		case <-ctx.Done():
			remove()
			return nil, ctx.Err()
		}
		return remove, nil
	}
	ip := id
	if chal.Type == "tls-alpn-01" {
		cert, err := m.client.TLSALPN01ChallengeCert(chal.Token, ip)
		if err != nil {
			return nil, err
		}
		m.mu.Lock()
		m.chal = &cert
		m.mu.Unlock()
		return func() {
			m.mu.Lock()
			m.chal = nil
			m.mu.Unlock()
		}, nil
	}
	resp, err := m.client.HTTP01ChallengeResponse(chal.Token)
	if err != nil {
		return nil, err
	}
	path := m.client.HTTP01ChallengePath(chal.Token)
	ln, err := net.Listen("tcp", m.httpAddr)
	if err != nil {
		return nil, fmt.Errorf("証明書の検証のために %s で待ち受けられません: %w", m.httpAddr, err)
	}
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(resp))
	})}
	go srv.Serve(ln)
	return func() { srv.Close() }, nil
}

// until は今の証明書の期限。まだ無ければ時刻のゼロ
func (m *pubCert) until() time.Time {
	if c := m.cur.Load(); c != nil && c.Leaf != nil {
		return c.Leaf.NotAfter
	}
	return time.Time{}
}

// needRenew は、証明書が無いか、有効期間の残りが半分を切ったか
func (m *pubCert) needRenew(now time.Time) bool {
	c := m.cur.Load()
	if c == nil || c.Leaf == nil {
		return true
	}
	life := c.Leaf.NotAfter.Sub(c.Leaf.NotBefore)
	return c.Leaf.NotAfter.Sub(now) < life/2
}

// GlobalIP は、この PC から外へ出るときのグローバル IP を Cloudflare の trace で調べる
func GlobalIP(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://1.1.1.1/cdn-cgi/trace", nil)
	if err != nil {
		return "", err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("グローバル IP を調べられません: %w", err)
	}
	defer res.Body.Close()
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "ip="); ok && net.ParseIP(v) != nil {
			return v, nil
		}
	}
	return "", errors.New("グローバル IP を調べられません")
}
