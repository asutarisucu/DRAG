package ovpn

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 試験用の OpenVPN のサーバー (dev/ovpnserver) に対して試す。
// DRAG_TEST_OVPN にサーバーが書いた .ovpn のフォルダ (dev/ovpnserver/out) を渡したときだけ動く
func loadConfig(t *testing.T, name string) *Config {
	dir := os.Getenv("DRAG_TEST_OVPN")
	if dir == "" {
		t.Skip("DRAG_TEST_OVPN が無い")
	}
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// icmpEcho は、from から to への ICMP の echo request を作る
func icmpEcho(from, to netip.Addr, seq uint16) []byte {
	icmp := []byte{8, 0, 0, 0, 0x12, 0x34, byte(seq >> 8), byte(seq)}
	icmp = append(icmp, "drag-ovpn-test"...)
	binary.BigEndian.PutUint16(icmp[2:], checksum(icmp))
	ip := []byte{0x45, 0, 0, 0, 0, 0, 0, 0, 64, 1, 0, 0}
	binary.BigEndian.PutUint16(ip[2:], uint16(20+len(icmp)))
	ip = append(ip, from.AsSlice()...)
	ip = append(ip, to.AsSlice()...)
	binary.BigEndian.PutUint16(ip[10:], checksum(ip))
	return append(ip, icmp...)
}

func checksum(b []byte) uint16 {
	var s uint32
	for i := 0; i+1 < len(b); i += 2 {
		s += uint32(b[i])<<8 | uint32(b[i+1])
	}
	if len(b)%2 == 1 {
		s += uint32(b[len(b)-1]) << 8
	}
	for s>>16 != 0 {
		s = s&0xffff + s>>16
	}
	return ^uint16(s)
}

// ping は VPN の中のサーバー (.1) に ICMP を送り、返事を待つ
func ping(t *testing.T, c *Client, seq uint16) {
	t.Helper()
	me := c.Pushed().Addrs[0]
	b := me.As4()
	b[3] = 1
	to := netip.AddrFrom4(b)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := c.WriteIP(icmpEcho(me, to, seq)); err != nil {
			t.Fatal(err)
		}
		got := make(chan []byte, 1)
		go func() {
			p, err := c.ReadIP()
			if err == nil {
				got <- p
			}
		}()
		select {
		case p := <-got:
			if len(p) >= 28 && p[9] == 1 && p[20] == 0 && binary.BigEndian.Uint16(p[26:]) == seq {
				return
			}
		case <-time.After(time.Second):
		}
	}
	t.Fatalf("%s から ICMP の返事がありません", to)
}

func dial(t *testing.T, name string, opts Options) *Client {
	t.Helper()
	cfg := loadConfig(t, name)
	opts.Logf = t.Logf
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, err := Dial(ctx, cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestOpenVPNTLSCryptUserPass(t *testing.T) {
	c := dial(t, "a.ovpn", Options{User: "asuta", Pass: "drag"})
	p := c.Pushed()
	if p.Cipher != "AES-256-GCM" || !p.Addrs[0].IsValid() || len(p.DNS) != 1 || p.DNS[0].String() != "10.8.0.1" {
		t.Fatalf("%+v", p)
	}
	ping(t, c, 1)
}

func TestOpenVPNWrongPassword(t *testing.T) {
	cfg := loadConfig(t, "a.ovpn")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := Dial(ctx, cfg, Options{User: "asuta", Pass: "wrong", Logf: t.Logf})
	if err == nil || !errors.Is(err, ErrAuth) {
		t.Fatalf("違うパスワードでつながった: %v", err)
	}
}

func TestOpenVPNTCPTLSAuth(t *testing.T) {
	c := dial(t, "b.ovpn", Options{})
	if c.Pushed().Cipher != "AES-128-GCM" {
		t.Fatalf("%+v", c.Pushed())
	}
	ping(t, c, 2)
}

func TestOpenVPNChaChaStubV2(t *testing.T) {
	c := dial(t, "c.ovpn", Options{})
	if c.Pushed().Cipher != "CHACHA20-POLY1305" {
		t.Fatalf("%+v", c.Pushed())
	}
	ping(t, c, 3)
}

func TestOpenVPNCBCWithOldPRF(t *testing.T) {
	c := dial(t, "d.ovpn", Options{NoEKM: true})
	if c.Pushed().Cipher != "AES-256-CBC" {
		t.Fatalf("%+v", c.Pushed())
	}
	ping(t, c, 4)
}

// a のサーバーは 30 秒ごとに鍵を替える。替えたあとも通る
func TestOpenVPNRenegotiation(t *testing.T) {
	if testing.Short() {
		t.Skip("40 秒かかる")
	}
	c := dial(t, "a.ovpn", Options{User: "asuta", Pass: "drag"})
	ping(t, c, 10)
	time.Sleep(40 * time.Second)
	c.mu.Lock()
	id := c.cur.id
	c.mu.Unlock()
	if id == 0 {
		t.Fatal("鍵が替わっていない")
	}
	ping(t, c, 11)
}
