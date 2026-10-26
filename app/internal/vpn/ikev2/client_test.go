package ikev2

import (
	"context"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 試験用の IKEv2 のサーバー (dev/ikev2server) に対して試す。
// DRAG_TEST_IKEV2 にサーバーの dev/ikev2server のフォルダを渡したときだけ動く。サーバーは 127.0.0.1:14500 で待つ
func testConfig(t *testing.T, pass string) Config {
	dir := os.Getenv("DRAG_TEST_IKEV2")
	if dir == "" {
		t.Skip("DRAG_TEST_IKEV2 が無い")
	}
	b, err := os.ReadFile(filepath.Join(dir, "out", "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(b) {
		t.Fatal("ca.crt を読めない")
	}
	return Config{Server: "127.0.0.1:14500", User: "asuta", Pass: pass, CA: pool, Logf: t.Logf}
}

func icmpEcho(from, to netip.Addr, seq uint16) []byte {
	icmp := []byte{8, 0, 0, 0, 0x12, 0x34, byte(seq >> 8), byte(seq)}
	icmp = append(icmp, "drag-ikev2-test"...)
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

// ping は VPN の中の 10.20.0.1 に ICMP を送り、返事を待つ
func ping(t *testing.T, c *Client, seq uint16) {
	t.Helper()
	me, to := c.Pushed().Addrs[0], netip.MustParseAddr("10.20.0.1")
	got := make(chan bool, 1)
	go func() {
		for {
			p, err := c.ReadIP()
			if err != nil {
				return
			}
			if len(p) >= 28 && p[9] == 1 && p[20] == 0 && binary.BigEndian.Uint16(p[26:]) == seq {
				got <- true
				return
			}
		}
	}()
	for i := 0; i < 5; i++ {
		if err := c.WriteIP(icmpEcho(me, to, seq)); err != nil {
			t.Fatal(err)
		}
		select {
		case <-got:
			return
		case <-time.After(time.Second):
		}
	}
	t.Fatalf("%s から ICMP の返事がありません", to)
}

func TestIKEv2EAP(t *testing.T) {
	cfg := testConfig(t, "drag")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, err := Dial(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p := c.Pushed()
	if !netip.MustParsePrefix("10.10.10.0/24").Contains(p.Addrs[0]) || len(p.DNS) != 1 || p.DNS[0].String() != "10.20.0.1" {
		t.Fatalf("%+v", p)
	}
	ping(t, c, 1)
}

func TestIKEv2WrongPassword(t *testing.T) {
	cfg := testConfig(t, "wrong")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := Dial(ctx, cfg)
	if err == nil || !errors.Is(err, errEAPFailed) {
		t.Fatalf("違うパスワードでつながった: %v", err)
	}
}

func TestIKEv2UnknownCA(t *testing.T) {
	cfg := testConfig(t, "drag")
	cfg.CA = x509.NewCertPool()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := Dial(ctx, cfg); err == nil {
		t.Fatal("知らない CA の証明書でつながった")
	}
}

// サーバーは ESP の SA を 40 秒ごと、IKE の SA を 50 秒ごとに替える。替えたあとも通る
func TestIKEv2Rekey(t *testing.T) {
	if testing.Short() {
		t.Skip("60 秒かかる")
	}
	cfg := testConfig(t, "drag")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, err := Dial(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ping(t, c, 10)
	c.mu.Lock()
	firstChild, firstIKE := c.child.inSPI, c.ike.spiI
	c.mu.Unlock()
	time.Sleep(60 * time.Second)
	ping(t, c, 11)
	c.mu.Lock()
	child, sa := c.child.inSPI, c.ike
	c.mu.Unlock()
	if child == firstChild {
		t.Fatal("ESP の SA が替わっていない")
	}
	if sa.spiI == firstIKE {
		t.Fatal("IKE の SA が替わっていない")
	}
	// 新しい IKE の SA で、生きているかの確かめが通る
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	if _, err := c.send(ctx2, sa, &message{exch: exInfo}); err != nil {
		t.Fatal(err)
	}
}

// IKE の SA の提案は PRF が無ければ選ばない。鍵を作る PRF が無いまま進むと nil の関数を呼ぶ
func TestPickProposalNeedsPRFForIKE(t *testing.T) {
	noPRF := proposal{num: 1, proto: protoIKE, spi: make([]byte, 8), transforms: []transform{{tEncr, encrAESGCM16, 128}, {tDH, 19, 0}}}
	if _, _, ok := pickProposal([]proposal{noPRF}, protoIKE); ok {
		t.Fatal("PRF の無い IKE の提案を選んだ")
	}
	withPRF := noPRF
	withPRF.transforms = append(withPRF.transforms, transform{tPRF, prfSHA256, 0})
	if _, s, ok := pickProposal([]proposal{withPRF}, protoIKE); !ok || s.prf != prfSHA256 {
		t.Fatal("PRF のある IKE の提案を選ばない")
	}
	// ESP の SA には PRF が無い
	esp := proposal{num: 1, proto: protoESP, spi: make([]byte, 4), transforms: []transform{{tEncr, encrAESGCM16, 128}, {tESN, 0, 0}}}
	if _, _, ok := pickProposal([]proposal{esp}, protoESP); !ok {
		t.Fatal("ESP の提案を選ばない")
	}
}

// 1024 ビットの MODP (グループ 2) は提案せず、受けもしない
func TestNoMODP1024(t *testing.T) {
	for _, p := range ikeProposals(nil) {
		for _, tr := range p.transforms {
			if tr.typ == tDH && tr.id == 2 {
				t.Fatal("グループ 2 を提案した")
			}
		}
	}
	if acceptable(transform{tDH, 2, 0}) {
		t.Fatal("グループ 2 を受ける")
	}
	if !acceptable(transform{tDH, 14, 0}) {
		t.Fatal("グループ 14 を受けない")
	}
}

// AEAD の IV は、同じ鍵で重ならないよう 1 ずつ増やす
func TestAEADIVsAreUnique(t *testing.T) {
	s := &suite{encr: encrAESGCM16, keyLen: 16}
	k := newCipherKeys(s, make([]byte, 20), nil)
	seen := map[string]bool{}
	for range 1000 {
		b, err := k.seal(func(int) []byte { return nil }, []byte("x"))
		if err != nil {
			t.Fatal(err)
		}
		if iv := string(b[:8]); seen[iv] {
			t.Fatal("IV が重なった")
		} else {
			seen[iv] = true
		}
	}
}
