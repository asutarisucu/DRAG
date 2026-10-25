package vpn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/asutarisucu/DRAG/app/internal/remote"
	"github.com/asutarisucu/DRAG/app/internal/store"
)

// 試験用の OpenVPN のサーバー (dev/ovpnserver) を通して、VPN の中の :22 から検証用の SSH サーバーへつなぐ。
// DRAG_TEST_OVPN にサーバーが書いた .ovpn のフォルダ (dev/ovpnserver/out) を渡したときだけ動く
func TestOpenVPNCarriesSSH(t *testing.T) {
	dir := os.Getenv("DRAG_TEST_OVPN")
	if dir == "" {
		t.Skip("DRAG_TEST_OVPN が無い")
	}
	for _, c := range []struct {
		file, server, user, pass string
	}{
		{"a.ovpn", "10.8.0.1", "asuta", "drag"},
		{"b.ovpn", "10.8.1.1", "", ""},
	} {
		b, err := os.ReadFile(filepath.Join(dir, c.file))
		if err != nil {
			t.Fatal(err)
		}
		v := store.VPN{Type: "ovpn", Config: string(b), User: c.user, Password: c.pass}
		sum, err := Check(v)
		if err != nil || sum.Login != (c.user != "") {
			t.Fatalf("%s: %+v %v", c.file, sum, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		tun, err := Open(ctx, v, c.server)
		cancel()
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if tun.Summary().Address == "" {
			t.Fatalf("%s: %+v", c.file, tun.Summary())
		}
		cfg := remote.Config{Host: c.server, Port: 22, User: "asuta", Password: "drag", Dial: tun.DialContext}
		kh := filepath.Join(t.TempDir(), "known_hosts")
		_, err = remote.Dial(cfg, kh, "")
		var he *remote.HostKeyError
		if !errors.As(err, &he) {
			t.Fatalf("%s: %v", c.file, err)
		}
		s, err := remote.Dial(cfg, kh, he.Fingerprint)
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if home, err := s.Home(); err != nil || home != "/home/asuta" {
			t.Fatalf("%s: %q %v", c.file, home, err)
		}
		s.Close()
		tun.Close()
		select {
		case <-tun.Done():
		case <-time.After(time.Second):
			t.Fatalf("%s: 閉じても Done が閉じない", c.file)
		}
	}
}

// 試験用の IKEv2 のサーバー (dev/ikev2server) を通して、VPN の中の 10.20.0.1:22 から検証用の SSH サーバーへつなぐ。
// DRAG_TEST_IKEV2 に dev/ikev2server のフォルダを渡したときだけ動く
func TestIKEv2CarriesSSH(t *testing.T) {
	dir := os.Getenv("DRAG_TEST_IKEV2")
	if dir == "" {
		t.Skip("DRAG_TEST_IKEV2 が無い")
	}
	ca, err := os.ReadFile(filepath.Join(dir, "out", "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	v := store.VPN{Type: "ikev2", Server: "127.0.0.1:14500", CA: string(ca), User: "asuta", Password: "drag"}
	if _, err := Check(store.VPN{Type: "ikev2", Server: "127.0.0.1:14500"}); err == nil {
		t.Fatal("パスワードが無いのに通った")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	tun, err := Open(ctx, v, "10.20.0.1")
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()
	if tun.Summary().Address == "" {
		t.Fatalf("%+v", tun.Summary())
	}
	cfg := remote.Config{Host: "10.20.0.1", Port: 22, User: "asuta", Password: "drag", Dial: tun.DialContext}
	kh := filepath.Join(t.TempDir(), "known_hosts")
	_, err = remote.Dial(cfg, kh, "")
	var he *remote.HostKeyError
	if !errors.As(err, &he) {
		t.Fatal(err)
	}
	s, err := remote.Dial(cfg, kh, he.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if home, err := s.Home(); err != nil || home != "/home/asuta" {
		t.Fatalf("%q %v", home, err)
	}
}
