package ovpn

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// nullTransport は送ったものを捨てる
type nullTransport struct{}

func (nullTransport) send([]byte) error     { return nil }
func (nullTransport) recv() ([]byte, error) { select {} }
func (nullTransport) close() error          { return nil }

// 窓より先の制御パケットは取っておかない。経路の途中から送り続けられてもメモリが増えない
func TestControlReorderWindow(t *testing.T) {
	w, _ := newWrapper(&Config{})
	c := &Client{tr: nullTransport{}, wrap: w, gotReset: make(chan struct{})}
	close(c.gotReset)
	ks := c.newKeyState(0)
	for i := uint32(1); i < 5000; i++ {
		c.handleControl(&ctrlPacket{op: opControl, keyID: 0, id: i, payload: make([]byte, 100)})
	}
	if len(ks.recvBuf) >= recvWindow {
		t.Fatalf("%d 個を取っておいた", len(ks.recvBuf))
	}
	// 窓の中なら、足りなかったものが届いたときに順に渡す
	c.handleControl(&ctrlPacket{op: opControl, keyID: 0, id: 0, payload: []byte("a")})
	if ks.recvNext != recvWindow || len(ks.recvBuf) != 0 {
		t.Fatalf("recvNext %d、残り %d", ks.recvNext, len(ks.recvBuf))
	}
}

// fragment の頭の size が .ovpn の値より大きいものは、バッファを取らずに捨てる
func TestFragmentSizeLimit(t *testing.T) {
	f := newFragmenter(1300)
	if got := f.join(append(putFrag(fragLast, 1, 31, 65532), make([]byte, 20)...)); got != nil || len(f.part) != 0 {
		t.Fatalf("大きすぎる size を受けた (%d 件)", len(f.part))
	}
	// 分けて送ったものは、つなぎ直せる
	p := make([]byte, 3000)
	for i := range p {
		p[i] = byte(i)
	}
	var got []byte
	for _, part := range f.split(p) {
		got = f.join(part)
	}
	if !bytes.Equal(got, p) {
		t.Fatalf("つなげない %d", len(got))
	}
}

// packet_id を使い切ったら、0 に戻して同じ nonce で暗号にせず、送るのをやめる
func TestSealStopsBeforeIDWraps(t *testing.T) {
	k, err := newDataKeys("AES-256-GCM", "SHA1", make([]byte, 256))
	if err != nil {
		t.Fatal(err)
	}
	k.nextID = math.MaxUint32 - 1
	if _, err := k.seal([]byte{opDataV1 << 3}, []byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := k.seal([]byte{opDataV1 << 3}, []byte("b")); err != errIDsUsedUp {
		t.Fatalf("%v", err)
	}
}

// 認証の合った制御パケットでも、前に受けた replay_packet_id のものは受けない
func TestControlReplay(t *testing.T) {
	w := &wrapper{}
	rid := func(id, at uint32) []byte {
		return binary.BigEndian.AppendUint32(binary.BigEndian.AppendUint32(nil, id), at)
	}
	for _, c := range []struct {
		id, at uint32
		replay bool
	}{
		{1, 100, false},
		{3, 100, false},
		{1, 100, true}, // 同じもの
		{2, 100, false},
		{9, 99, true}, // 時刻が戻った
		{1, 101, false},
	} {
		if got := w.replayed(rid(c.id, c.at)); got != c.replay {
			t.Errorf("%d@%d: %v", c.id, c.at, got)
		}
	}
}
