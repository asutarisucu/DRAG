package phone

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// 直接公開の証明書を、pebble (Let's Encrypt の試験用のサーバー) から取る。
// DRAG_TEST_PEBBLE に pebble のディレクトリの URL、DRAG_TEST_PEBBLE_CA に pebble の HTTPS の CA の PEM を渡したときだけ動く。
// pebble は httpPort を 18080 にして動かしておく
func TestPublicRouteGetsIPCertificate(t *testing.T) {
	dir, caPath := os.Getenv("DRAG_TEST_PEBBLE"), os.Getenv("DRAG_TEST_PEBBLE_CA")
	if dir == "" || caPath == "" {
		t.Skip("DRAG_TEST_PEBBLE が無い")
	}
	ca, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca)
	acmeClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}

	s := New(Options{
		Dir:    t.TempDir(),
		Assets: fstest.MapFS{"index.html": {Data: []byte("<title>app</title>")}},
		Call:   func(Device, string, []json.RawMessage) (any, error) { return nil, nil },
	})
	t.Cleanup(s.Stop)
	if err := s.StartPublic(PublicConfig{Port: 18446, IP: "127.0.0.1", ACME: dir, Client: acmeClient, HTTPPort: 18080, Host: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for s.Status(RoutePublic).URL == "" {
		if st := s.Status(RoutePublic); st.Error != "" {
			t.Fatal(st.Error)
		}
		if time.Now().After(deadline) {
			t.Fatal("証明書が取れない")
		}
		time.Sleep(200 * time.Millisecond)
	}
	st := s.Status(RoutePublic)
	if st.URL != "https://127.0.0.1:18446" || st.Pending || time.Until(time.UnixMilli(st.CertUntil)) < 100*time.Hour {
		t.Fatalf("%+v", st)
	}

	// 待ち受けは、取った証明書を出す
	conn, err := tls.Dial("tcp", "127.0.0.1:18446", &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	leaf := conn.ConnectionState().PeerCertificates[0]
	conn.Close()
	if len(leaf.IPAddresses) != 1 || !leaf.IPAddresses[0].Equal(net.ParseIP("127.0.0.1")) || !strings.Contains(leaf.Issuer.CommonName, "Pebble") {
		t.Fatalf("%v %s", leaf.IPAddresses, leaf.Issuer.CommonName)
	}

	// 直接公開で登録した端末は、直接公開でだけ入れる
	u, _, err := s.NewPairURL(RoutePublic)
	if err != nil || !strings.HasPrefix(u, "https://127.0.0.1:18446/pair?t=") {
		t.Fatalf("%q %v", u, err)
	}
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{Jar: jar, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	if code, _ := get(t, hc, u); code != http.StatusOK {
		t.Fatalf("%d", code)
	}
	if d := s.Devices(); len(d) != 1 || d[0].Route != RoutePublic {
		t.Fatalf("%+v", d)
	}
	if _, _, err := s.NewPairURL(RouteLAN); err == nil {
		t.Fatal("LAN の待ち受けが止まっているのに QR コードができた")
	}
}
