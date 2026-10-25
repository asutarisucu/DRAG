package sstp

import (
	"context"
	"crypto/x509"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/asutarisucu/DRAG/app/internal/vpn/ppp"
)

// 試験用の SoftEther VPN (dev/softether) に対して試す。
// DRAG_TEST_SOFTETHER に dev/softether のフォルダを渡したときだけ動く。SSTP は 127.0.0.1:14443 で待つ
func testConfig(t *testing.T, pass string) Config {
	dir := os.Getenv("DRAG_TEST_SOFTETHER")
	if dir == "" {
		t.Skip("DRAG_TEST_SOFTETHER が無い")
	}
	b, err := os.ReadFile(filepath.Join(dir, "out", "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(b) {
		t.Fatal("ca.crt を読めない")
	}
	return Config{Server: "127.0.0.1:14443", User: "asuta", Pass: pass, CA: pool, Logf: t.Logf}
}

func TestSSTP(t *testing.T) {
	cfg := testConfig(t, "drag")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := Dial(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if p := c.Pushed(); !netip.MustParsePrefix("192.168.30.0/24").Contains(p.Addr) {
		t.Fatalf("%+v", p)
	}
}

func TestSSTPWrongPassword(t *testing.T) {
	cfg := testConfig(t, "wrong")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := Dial(ctx, cfg); err == nil || !errors.Is(err, ppp.ErrAuth) {
		t.Fatalf("違うパスワードでつながった: %v", err)
	}
}

func TestSSTPUnknownCA(t *testing.T) {
	cfg := testConfig(t, "drag")
	cfg.CA = x509.NewCertPool()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := Dial(ctx, cfg); err == nil {
		t.Fatal("知らない CA の証明書でつながった")
	}
}
