package phone

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"slices"
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

// challtestsrv は、pebble に付いている試験用の DNS (pebble-challtestsrv) に TXT レコードを置く。
// 管理の API は pebble v2.10.1 cmd/pebble-challtestsrv の /set-txt と /clear-txt
type challtestsrv string

func (c challtestsrv) SetTXT(ctx context.Context, name, value string) (func(), error) {
	post := func(path string, body map[string]string) error {
		b, _ := json.Marshal(body)
		res, err := http.Post(string(c)+path, "application/json", bytes.NewReader(b))
		if err != nil {
			return err
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("%s: %s", path, res.Status)
		}
		return nil
	}
	if err := post("/set-txt", map[string]string{"host": name, "value": value}); err != nil {
		return nil, err
	}
	return func() { post("/clear-txt", map[string]string{"host": name}) }, nil
}

// 自分のドメインの証明書を、pebble から DNS-01 で取る。
// TestPublicRouteGetsIPCertificate の用意に加え、DRAG_TEST_CHALLTESTSRV に pebble-challtestsrv の管理の URL を渡したときだけ動く。
// pebble は -dnsserver で pebble-challtestsrv の DNS を指しておく
func TestPublicRouteGetsDomainCertificate(t *testing.T) {
	dir, caPath, chall := os.Getenv("DRAG_TEST_PEBBLE"), os.Getenv("DRAG_TEST_PEBBLE_CA"), os.Getenv("DRAG_TEST_CHALLTESTSRV")
	if dir == "" || caPath == "" || chall == "" {
		t.Skip("DRAG_TEST_CHALLTESTSRV が無い")
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
	cfg := PublicConfig{Port: 18447, IP: "127.0.0.1", ACME: dir, Client: acmeClient, Host: "127.0.0.1",
		Domain: "drag.test", DNS: challtestsrv(chall), DNSWait: time.Millisecond}
	if err := s.StartPublic(cfg); err != nil {
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
	// profile を付けない注文には、pebble v2.10.1 wfe/wfe.go は profile を乱数で選ぶので、期間の長さは見ない。
	// drag.test は名前を引けないので、向き先の注意が出る
	if st.URL != "https://drag.test:18447" || time.Until(time.UnixMilli(st.CertUntil)) < 100*time.Hour ||
		!strings.Contains(st.Warn, "drag.test がこの PC のグローバル IP (127.0.0.1) を指していません") {
		t.Fatalf("%+v", st)
	}
	conn, err := tls.Dial("tcp", "127.0.0.1:18447", &tls.Config{InsecureSkipVerify: true, ServerName: "drag.test"})
	if err != nil {
		t.Fatal(err)
	}
	leaf := conn.ConnectionState().PeerCertificates[0]
	conn.Close()
	if !slices.Equal(leaf.DNSNames, []string{"drag.test"}) || len(leaf.IPAddresses) != 0 {
		t.Fatalf("%v %v", leaf.DNSNames, leaf.IPAddresses)
	}
}
