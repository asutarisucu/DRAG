package vpn

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"

	"github.com/asutarisucu/DRAG/app/internal/remote"
	"github.com/asutarisucu/DRAG/app/internal/store"
)

func newKey(t *testing.T) (priv, pub []byte) {
	priv = make([]byte, 32)
	rand.Read(priv)
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub
}

// loopBind は、試験の相手の側の conn.Bind。127.0.0.1 だけで待ち受ける
type loopBind struct {
	c    *net.UDPConn
	port uint16
}

func (b *loopBind) Open(uint16) ([]conn.ReceiveFunc, uint16, error) {
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, 0, err
	}
	b.c, b.port = c, uint16(c.LocalAddr().(*net.UDPAddr).Port)
	fn := func(packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		n, from, err := c.ReadFromUDPAddrPort(packets[0])
		if err != nil {
			return 0, err
		}
		sizes[0], eps[0] = n, udpEndpoint(from)
		return 1, nil
	}
	return []conn.ReceiveFunc{fn}, b.port, nil
}
func (b *loopBind) Close() error {
	if b.c != nil {
		return b.c.Close()
	}
	return nil
}
func (b *loopBind) SetMark(uint32) error { return nil }
func (b *loopBind) BatchSize() int       { return 1 }
func (b *loopBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	ap, err := netip.ParseAddrPort(s)
	return udpEndpoint(ap), err
}
func (b *loopBind) Send(bufs [][]byte, ep conn.Endpoint) error {
	for _, buf := range bufs {
		if _, err := b.c.WriteToUDPAddrPort(buf, netip.AddrPort(ep.(udpEndpoint))); err != nil {
			return err
		}
	}
	return nil
}

// wgServer は試験の相手の WireGuard。VPN の中の 10.99.0.1 で待ち受ける
type wgServer struct {
	net  *netstack.Net
	pub  []byte
	port uint16
}

func startServer(t *testing.T, clientPub []byte) *wgServer {
	priv, pub := newKey(t)
	tdev, tnet, err := netstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr("10.99.0.1")}, nil, 1420)
	if err != nil {
		t.Fatal(err)
	}
	bind := &loopBind{}
	dev := device.NewDevice(tdev, bind, &device.Logger{Verbosef: device.DiscardLogf, Errorf: device.DiscardLogf})
	t.Cleanup(dev.Close)
	if err := dev.IpcSet(fmt.Sprintf("private_key=%s\npublic_key=%s\nallowed_ip=10.99.0.2/32\n", hex.EncodeToString(priv), hex.EncodeToString(clientPub))); err != nil {
		t.Fatal(err)
	}
	if err := dev.Up(); err != nil {
		t.Fatal(err)
	}
	return &wgServer{net: tnet, pub: pub, port: bind.port}
}

// serve は、VPN の中の port で受けたものを handle に渡す
func (s *wgServer) serve(t *testing.T, port uint16, handle func(net.Conn)) {
	l, err := s.net.ListenTCPAddrPort(netip.AddrPortFrom(netip.MustParseAddr("10.99.0.1"), port))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go handle(c)
		}
	}()
}

func clientConfig(priv, serverPub []byte, port uint16) string {
	b64 := base64.StdEncoding.EncodeToString
	return fmt.Sprintf(`# 試験用
[Interface]
PrivateKey = %s
Address = 10.99.0.2/32

[Peer]
PublicKey = %s
Endpoint = 127.0.0.1:%d
AllowedIPs = 10.99.0.0/24
`, b64(priv), b64(serverPub), port)
}

func TestWireGuardCarriesTCP(t *testing.T) {
	priv, pub := newKey(t)
	srv := startServer(t, pub)
	srv.serve(t, 7, func(c net.Conn) { io.Copy(c, c); c.Close() })

	v := store.VPN{Type: "wg", Config: clientConfig(priv, srv.pub, srv.port)}
	sum, err := Check(v)
	if err != nil || sum.Target != fmt.Sprintf("127.0.0.1:%d", srv.port) || sum.Address != "10.99.0.2" || sum.Routes != "10.99.0.0/24" {
		t.Fatalf("%+v %v", sum, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tun, err := Open(ctx, v, "10.99.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()

	c, err := tun.DialContext(ctx, "tcp", "10.99.0.1:7")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Write([]byte("hello"))
	buf := make([]byte, 5)
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "hello" {
		t.Fatalf("%q %v", buf, err)
	}
	// 通す範囲の外へは、VPN を通さない
	if _, err := tun.DialContext(ctx, "tcp", "192.0.2.1:22"); err == nil || !strings.Contains(err.Error(), "AllowedIPs") {
		t.Fatalf("範囲の外につないだ: %v", err)
	}
}

func TestWireGuardReportsNoHandshake(t *testing.T) {
	priv, pub := newKey(t)
	srv := startServer(t, pub)
	_, wrong := newKey(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := Open(ctx, store.VPN{Type: "wg", Config: clientConfig(priv, wrong, srv.port)}, "10.99.0.1")
	if err == nil || !strings.Contains(err.Error(), "返事がありません") {
		t.Fatalf("相手の鍵が違うのに張れた: %v", err)
	}
}

func TestParseWireGuardErrors(t *testing.T) {
	for conf, want := range map[string]string{
		"[Interface]\nAddress = 10.0.0.2/32\n[Peer]\n":                                        "PrivateKey",
		"[Interface]\nPrivateKey = abc\n":                                                     "鍵の形",
		"[Interface]\nPrivateKey = " + strings.Repeat("A", 43) + "=\n":                        "Address",
		"[Interface]\nPrivateKey = " + strings.Repeat("A", 43) + "=\nAddress = 10.0.0.2/32\n": "[Peer]",
	} {
		if _, err := parseWireGuard(conf); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", conf, err)
		}
	}
}

// 検証用のサーバー (dev/testserver) に、VPN の中から SSH でつなぐ。
// DRAG_TEST_SSH=127.0.0.1:2222 のように接続先を渡したときだけ動く
func TestWireGuardCarriesSSH(t *testing.T) {
	addr := os.Getenv("DRAG_TEST_SSH")
	if addr == "" {
		t.Skip("DRAG_TEST_SSH が無い")
	}
	priv, pub := newKey(t)
	srv := startServer(t, pub)
	// VPN の中の 10.99.0.1:22 を、検証用のサーバーへ渡す
	srv.serve(t, 22, func(c net.Conn) {
		defer c.Close()
		up, err := net.Dial("tcp", addr)
		if err != nil {
			return
		}
		defer up.Close()
		go io.Copy(up, c)
		io.Copy(c, up)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tun, err := Open(ctx, store.VPN{Type: "wg", Config: clientConfig(priv, srv.pub, srv.port)}, "10.99.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer tun.Close()

	cfg := remote.Config{Host: "10.99.0.1", Port: 22, User: "asuta", Password: "drag", Dial: tun.DialContext}
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
