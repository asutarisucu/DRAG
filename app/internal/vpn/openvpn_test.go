package vpn

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// 試験用の SSTP のサーバー (dev/softether と dev/accelppp) を通して、VPN の中から検証用の SSH サーバーへつなぐ。
// DRAG_TEST_SOFTETHER と DRAG_TEST_ACCEL に、それぞれのフォルダを渡したときだけ動く。
// accel-ppp は Crypto Binding の Compound MAC を確かめる
func TestSSTPCarriesSSH(t *testing.T) {
	for _, s := range []struct {
		env, server, target string
	}{
		{"DRAG_TEST_SOFTETHER", "127.0.0.1:14443", ""},
		{"DRAG_TEST_ACCEL", "127.0.0.1:15443", "192.168.40.1"},
	} {
		dir := os.Getenv(s.env)
		if dir == "" {
			t.Logf("%s が無いので飛ばす", s.env)
			continue
		}
		ca, err := os.ReadFile(filepath.Join(dir, "out", "ca.crt"))
		if err != nil {
			t.Fatal(err)
		}
		target := s.target
		if target == "" {
			b, err := os.ReadFile(filepath.Join(dir, "out", "target"))
			if err != nil {
				t.Fatal(err)
			}
			target = strings.TrimSpace(string(b))
		}
		v := store.VPN{Type: "sstp", Server: s.server, CA: string(ca), User: "asuta", Password: "drag"}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		tun, err := Open(ctx, v, target)
		cancel()
		if err != nil {
			t.Fatalf("%s: %v", s.env, err)
		}
		cfg := remote.Config{Host: target, Port: 22, User: "asuta", Password: "drag", Dial: tun.DialContext}
		kh := filepath.Join(t.TempDir(), "known_hosts")
		_, err = remote.Dial(cfg, kh, "")
		var he *remote.HostKeyError
		if !errors.As(err, &he) {
			t.Fatalf("%s: %v", s.env, err)
		}
		sc, err := remote.Dial(cfg, kh, he.Fingerprint)
		if err != nil {
			t.Fatalf("%s: %v", s.env, err)
		}
		if home, err := sc.Home(); err != nil || home != "/home/asuta" {
			t.Fatalf("%s: %q %v", s.env, home, err)
		}
		sc.Close()
		tun.Close()
	}
}

// 試験用の ocserv (dev/ocserv) を通して、VPN の中の 192.168.50.1:22 から検証用の SSH サーバーへつなぐ。
// DRAG_TEST_OCSERV に dev/ocserv のフォルダを渡したときだけ動く
func TestAnyConnectCarriesSSH(t *testing.T) {
	dir := os.Getenv("DRAG_TEST_OCSERV")
	if dir == "" {
		t.Skip("DRAG_TEST_OCSERV が無い")
	}
	ca, err := os.ReadFile(filepath.Join(dir, "out", "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	v := store.VPN{Type: "ac", Server: "127.0.0.1:16443", CA: string(ca), Group: "ops", User: "asuta", Password: "drag"}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	tun, err := Open(ctx, v, "192.168.50.1")
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()
	if s := tun.Summary(); s.Routes != "192.168.50.0/24" {
		t.Fatalf("%+v", s)
	}
	cfg := remote.Config{Host: "192.168.50.1", Port: 22, User: "asuta", Password: "drag", Dial: tun.DialContext}
	kh := filepath.Join(t.TempDir(), "known_hosts")
	_, err = remote.Dial(cfg, kh, "")
	var he *remote.HostKeyError
	if !errors.As(err, &he) {
		t.Fatal(err)
	}
	sc, err := remote.Dial(cfg, kh, he.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Close()
	if home, err := sc.Home(); err != nil || home != "/home/asuta" {
		t.Fatalf("%q %v", home, err)
	}
}

// 試験用の SoftEther VPN (dev/softether) に L2TP/IPsec でつなぎ、VPN の中から検証用の SSH サーバーへつなぐ。
// DRAG_TEST_SOFTETHER に dev/softether のフォルダを渡したときだけ動く
func TestL2TPCarriesSSH(t *testing.T) {
	dir := os.Getenv("DRAG_TEST_SOFTETHER")
	if dir == "" {
		t.Skip("DRAG_TEST_SOFTETHER が無い")
	}
	b, err := os.ReadFile(filepath.Join(dir, "out", "target"))
	if err != nil {
		t.Fatal(err)
	}
	target := strings.TrimSpace(string(b))
	v := store.VPN{Type: "l2tp", Server: "127.0.0.1:15501", PSK: "dragpsk", User: "asuta", Password: "drag"}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	tun, err := Open(ctx, v, target)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()
	cfg := remote.Config{Host: target, Port: 22, User: "asuta", Password: "drag", Dial: tun.DialContext}
	kh := filepath.Join(t.TempDir(), "known_hosts")
	_, err = remote.Dial(cfg, kh, "")
	var he *remote.HostKeyError
	if !errors.As(err, &he) {
		t.Fatal(err)
	}
	sc, err := remote.Dial(cfg, kh, he.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	defer sc.Close()
	if home, err := sc.Home(); err != nil || home != "/home/asuta" {
		t.Fatalf("%q %v", home, err)
	}
}

// 試験用の OpenVPN サーバー (dev/ovpnserver) の e、f、g は、サーバーが LZO と LZ4 で圧縮して送ってくる。
// SSH の鍵の交換と、よく縮むコマンドの出力が、戻した上で届くことを確かめる
func TestOpenVPNCompressed(t *testing.T) {
	dir := os.Getenv("DRAG_TEST_OVPN")
	if dir == "" {
		t.Skip("DRAG_TEST_OVPN が無い")
	}
	var want strings.Builder
	for i := 1; i <= 20000; i++ {
		fmt.Fprintf(&want, "%d\n", i)
	}
	for _, c := range []struct{ file, server string }{
		{"e.ovpn", "10.8.4.1"},
		{"f.ovpn", "10.8.5.1"},
		{"g.ovpn", "10.8.6.1"},
	} {
		b, err := os.ReadFile(filepath.Join(dir, c.file))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		tun, err := Open(ctx, store.VPN{Type: "ovpn", Config: string(b)}, c.server)
		cancel()
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
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
		r, err := s.Exec("seq 1 20000", 20*time.Second)
		if err != nil || r.Stdout != want.String() {
			t.Fatalf("%s: %d バイト (欲しいのは %d) %v", c.file, len(r.Stdout), want.Len(), err)
		}
		s.Close()
		tun.Close()
	}
}
