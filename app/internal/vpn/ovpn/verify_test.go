package ovpn

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"
)

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  string
	next int64
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return &testCA{cert: cert, key: key, pem: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), next: 2}
}

// issue は CA で証明書を出す。f で用途を決める
func (ca *testCA) issue(t *testing.T, cn string, f func(*x509.Certificate)) *x509.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(ca.next), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	ca.next++
	if f != nil {
		f(tmpl)
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return cert
}

func (ca *testCA) crl(t *testing.T, revoked ...*x509.Certificate) string {
	t.Helper()
	tmpl := &x509.RevocationList{Number: big.NewInt(1), ThisUpdate: time.Now().Add(-time.Hour), NextUpdate: time.Now().Add(time.Hour)}
	for _, c := range revoked {
		tmpl.RevokedCertificateEntries = append(tmpl.RevokedCertificateEntries, x509.RevocationListEntry{SerialNumber: c.SerialNumber, RevocationTime: time.Now()})
	}
	der, err := x509.CreateRevocationList(rand.Reader, tmpl, ca.cert, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: der}))
}

func serverEKU(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth} }
func clientEKU(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth} }

func verifyWith(t *testing.T, conf string, cert *x509.Certificate) error {
	t.Helper()
	cfg, err := Parse(conf)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return (&Client{cfg: cfg}).verifyServer([][]byte{cert.Raw}, nil)
}

func fingerprint(der []byte) string {
	h := sha256.Sum256(der)
	parts := make([]string, len(h))
	for i, b := range h {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}

// サーバーの用途をはっきり持つ証明書だけをサーバーとして通す。同じ CA のクライアント証明書や、用途の無い証明書は通さない
func TestVerifyServerNeedsServerPurpose(t *testing.T) {
	ca := newTestCA(t)
	base := "client\nremote 127.0.0.1\nauth-user-pass\n<ca>\n" + ca.pem + "</ca>\n"
	nsServer := func(c *x509.Certificate) {
		v, _ := asn1.Marshal(asn1.BitString{Bytes: []byte{0x40}, BitLength: 2})
		c.ExtraExtensions = []pkix.Extension{{Id: oidNSCertType, Value: v}}
	}
	for _, c := range []struct {
		name string
		conf string
		f    func(*x509.Certificate)
		ok   bool
	}{
		{"サーバー", base, serverEKU, true},
		{"クライアント", base, clientEKU, false},
		{"用途なし", base, nil, false},
		{"nsCertType の server", base, nsServer, true},
		{"remote-cert-tls でもクライアントは通さない", base + "remote-cert-tls server\n", clientEKU, false},
		{"remote-cert-tls は key usage を求める", base + "remote-cert-tls server\n", func(c *x509.Certificate) { serverEKU(c); c.KeyUsage = 0 }, false},
		{"TLS に使えない key usage", base, func(c *x509.Certificate) { serverEKU(c); c.KeyUsage = x509.KeyUsageCertSign }, false},
		{"remote-cert-ku が合う", base + "remote-cert-ku 80\n", serverEKU, true},
		{"remote-cert-ku が合わない", base + "remote-cert-ku a0 88\n", serverEKU, false},
		{"remote-cert-eku が合う", base + `remote-cert-eku "TLS Web Server Authentication"` + "\n", serverEKU, true},
		{"remote-cert-eku が合わない", base + "remote-cert-eku 1.3.6.1.5.5.7.3.9\n", serverEKU, false},
	} {
		err := verifyWith(t, c.conf, ca.issue(t, "server", c.f))
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestVerifyServerHashes(t *testing.T) {
	ca := newTestCA(t)
	other := newTestCA(t)
	srv := ca.issue(t, "server", serverEKU)
	base := "client\nremote 127.0.0.1\nauth-user-pass\n<ca>\n" + ca.pem + "</ca>\n"
	if err := verifyWith(t, base+"verify-hash "+fingerprint(ca.cert.Raw)+" SHA256\n", srv); err != nil {
		t.Errorf("CA の指紋が合うのに断った: %v", err)
	}
	if err := verifyWith(t, base+"<verify-hash>\n"+fingerprint(other.cert.Raw)+"\n</verify-hash>\n", srv); err == nil {
		t.Error("違う CA の指紋で通した")
	}
	// peer-fingerprint だけなら CA なしで、自分で署名した証明書を通す
	self := other.cert
	pinned := "client\nremote 127.0.0.1\nauth-user-pass\npeer-fingerprint " + fingerprint(self.Raw) + "\n"
	if err := verifyWith(t, pinned, self); err != nil {
		t.Errorf("指紋が合うのに断った: %v", err)
	}
	if err := verifyWith(t, pinned, srv); err == nil {
		t.Error("指紋が違うのに通した")
	}
}

func TestVerifyServerCRL(t *testing.T) {
	ca := newTestCA(t)
	good := ca.issue(t, "server", serverEKU)
	bad := ca.issue(t, "server", serverEKU)
	conf := "client\nremote 127.0.0.1\nauth-user-pass\n<ca>\n" + ca.pem + "</ca>\n<crl-verify>\n" + ca.crl(t, bad) + "</crl-verify>\n"
	if err := verifyWith(t, conf, good); err != nil {
		t.Errorf("失効していないのに断った: %v", err)
	}
	if err := verifyWith(t, conf, bad); err == nil || !strings.Contains(err.Error(), "失効") {
		t.Errorf("失効した証明書: %v", err)
	}
}

// 確かめ方を変える項目で、扱えないものは黙って捨てずに断る
func TestParseRejectsUnsupportedVerifyOptions(t *testing.T) {
	ca := newTestCA(t)
	base := "client\nremote 127.0.0.1\nauth-user-pass\n<ca>\n" + ca.pem + "</ca>\n"
	for _, line := range []string{"tls-verify /bin/check", "crl-verify crl.pem", "crl-verify /etc/crl dir", "ns-cert-type client", "remote-cert-tls peer", "remote-cert-eku codeSigning"} {
		if _, err := Parse(base + line + "\n"); err == nil {
			t.Errorf("%s を受け付けた", line)
		}
	}
}
