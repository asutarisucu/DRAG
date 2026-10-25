package l2tp

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/asutarisucu/DRAG/app/internal/vpn/ppp"
)

// 試験用の SoftEther VPN (dev/softether) に対して試す。
// DRAG_TEST_SOFTETHER に dev/softether のフォルダを渡したときだけ動く。NAT-T は 127.0.0.1:15501、事前共有キーは dragpsk
func testConfig(t *testing.T, psk, pass string) Config {
	if os.Getenv("DRAG_TEST_SOFTETHER") == "" {
		t.Skip("DRAG_TEST_SOFTETHER が無い")
	}
	return Config{Server: "127.0.0.1:15501", PSK: psk, User: "asuta", Pass: pass, Logf: t.Logf}
}

func TestL2TP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	c, err := Dial(ctx, testConfig(t, "dragpsk", "drag"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if p := c.Pushed(); !netip.MustParsePrefix("192.168.30.0/24").Contains(p.Addr) {
		t.Fatalf("%+v", p)
	}
}

func TestL2TPWrongPSK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	if _, err := Dial(ctx, testConfig(t, "wrong", "drag")); err == nil || !strings.Contains(err.Error(), "事前共有キー") {
		t.Fatalf("違う事前共有キーでつながった: %v", err)
	}
}

func TestL2TPWrongPassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	if _, err := Dial(ctx, testConfig(t, "dragpsk", "wrong")); !errors.Is(err, ppp.ErrAuth) {
		t.Fatalf("違うパスワードでつながった: %v", err)
	}
}

// ESP の SA の寿命を短くして、作り直しても VPN が続くことを確かめる。
// 自分で作り直す場合と、サーバーが始める Quick Mode に答える場合の両方を試す
func TestL2TPRekey(t *testing.T) {
	if testing.Short() {
		t.Skip("時間がかかる")
	}
	for _, server := range []bool{false, true} {
		cfg := testConfig(t, "dragpsk", "drag")
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		c, err := dial(ctx, cfg, 30*time.Second, server)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(75 * time.Second)
		c.mu.Lock()
		n := len(c.outbound) + len(c.later)
		c.mu.Unlock()
		select {
		case <-c.Done():
			t.Fatalf("サーバーが始める=%v: 切れた: %v", server, c.Err())
		default:
		}
		if n == 0 {
			t.Fatalf("サーバーが始める=%v: 送る SA が無い", server)
		}
		// L2TP の HELLO が通るか
		c.sendControl(0, avp(avpMessageType, u16(msgHello)))
		time.Sleep(3 * time.Second)
		c.mu.Lock()
		left := len(c.unacked)
		c.mu.Unlock()
		if left != 0 {
			t.Fatalf("サーバーが始める=%v: HELLO の返事が無い", server)
		}
		c.Close()
	}
}
