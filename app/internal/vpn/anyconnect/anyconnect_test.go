package anyconnect

import (
	"context"
	"crypto/x509"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 試験用の ocserv (dev/ocserv) に対して試す。
// DRAG_TEST_OCSERV に dev/ocserv のフォルダを渡したときだけ動く。127.0.0.1:16443 で待ち、グループは dev と ops
func testConfig(t *testing.T, pass string) Config {
	dir := os.Getenv("DRAG_TEST_OCSERV")
	if dir == "" {
		t.Skip("DRAG_TEST_OCSERV が無い")
	}
	b, err := os.ReadFile(filepath.Join(dir, "out", "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(b) {
		t.Fatal("ca.crt を読めない")
	}
	return Config{Server: "127.0.0.1:16443", User: "asuta", Pass: pass, CA: pool, MTU: 1400, Logf: t.Logf}
}

func dial(t *testing.T, cfg Config) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return Dial(ctx, cfg)
}

func TestAnyConnect(t *testing.T) {
	for _, g := range []string{"", "ops"} {
		cfg := testConfig(t, "drag")
		cfg.Group = g
		c, err := dial(t, cfg)
		if err != nil {
			t.Fatalf("グループ %q: %v", g, err)
		}
		p := c.Pushed()
		t.Logf("グループ %q: %+v", g, p)
		if !netip.MustParsePrefix("192.168.50.0/24").Contains(p.Addr) || len(p.DNS) != 1 || len(p.Routes) != 1 || p.Routes[0].String() != "192.168.50.0/24" {
			t.Fatalf("%+v", p)
		}
		c.Close()
	}
}

func TestAnyConnectWrongPassword(t *testing.T) {
	if _, err := dial(t, testConfig(t, "wrong")); !errors.Is(err, ErrAuth) {
		t.Fatalf("違うパスワードでつながった: %v", err)
	}
}

func TestAnyConnectUnknownGroup(t *testing.T) {
	cfg := testConfig(t, "drag")
	cfg.Group = "sales"
	if _, err := dial(t, cfg); err == nil || !strings.Contains(err.Error(), "グループ sales") {
		t.Fatalf("無いグループでつながった: %v", err)
	}
}

func TestAnyConnectUnknownCA(t *testing.T) {
	cfg := testConfig(t, "drag")
	cfg.CA = x509.NewCertPool()
	if _, err := dial(t, cfg); err == nil {
		t.Fatal("知らない CA の証明書でつながった")
	}
}

func TestParseRoute(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.50.0/255.255.255.0": "192.168.50.0/24",
		"10.0.0.0/8":                 "10.0.0.0/8",
	} {
		if r, ok := parseRoute(in); !ok || r.String() != want {
			t.Fatalf("%s: %v %v", in, r, ok)
		}
	}
	if _, ok := parseRoute("fd00::/64"); ok {
		t.Fatal("IPv6 を読んだ")
	}
}
