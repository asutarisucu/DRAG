package ovpn

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"hash"
	"sync"
	"time"
)

// opcode。パケットの先頭のバイトの上の 5 ビット。下の 3 ビットは key_id
const (
	opSoftReset         = 3
	opControl           = 4
	opAck               = 5
	opDataV1            = 6
	opHardResetClientV2 = 7
	opHardResetServerV2 = 8
	opDataV2            = 9
)

func isControl(op byte) bool {
	return op == opSoftReset || op == opControl || op == opAck || op == opHardResetClientV2 || op == opHardResetServerV2
}

var digests = map[string]func() hash.Hash{
	"SHA1": sha1.New, "SHA256": sha256.New, "SHA384": sha512.New384, "SHA512": sha512.New,
}

// ctrlPacket は制御チャネルのパケット1つ。包み (tls-auth や tls-crypt) を外したあとの中身
type ctrlPacket struct {
	op, keyID byte
	sid       [8]byte
	acks      []uint32
	peerSid   [8]byte // acks があるときだけ
	id        uint32  // ACK_V1 には無い
	payload   []byte
}

// body は、opcode と session_id のあとに続く部分。包みはこの部分を守る
func (p *ctrlPacket) body() []byte {
	b := []byte{byte(len(p.acks))}
	for _, a := range p.acks {
		b = binary.BigEndian.AppendUint32(b, a)
	}
	if len(p.acks) > 0 {
		b = append(b, p.peerSid[:]...)
	}
	if p.op != opAck {
		b = binary.BigEndian.AppendUint32(b, p.id)
	}
	return append(b, p.payload...)
}

func parseBody(p *ctrlPacket, b []byte) error {
	if len(b) < 1 {
		return errShort
	}
	n := int(b[0])
	b = b[1:]
	if len(b) < n*4 {
		return errShort
	}
	for i := 0; i < n; i++ {
		p.acks = append(p.acks, binary.BigEndian.Uint32(b[i*4:]))
	}
	b = b[n*4:]
	if n > 0 {
		if len(b) < 8 {
			return errShort
		}
		copy(p.peerSid[:], b)
		b = b[8:]
	}
	if p.op != opAck {
		if len(b) < 4 {
			return errShort
		}
		p.id = binary.BigEndian.Uint32(b)
		b = b[4:]
	}
	p.payload = b
	return nil
}

var errShort = errors.New("パケットが短すぎます")

// wrapper は制御チャネルの包み。tls-auth は HMAC を付け、tls-crypt は暗号にする。どちらも無ければそのまま送る
type wrapper struct {
	mu    sync.Mutex
	next  uint32 // 送るたびに増やす replay_packet_id の下の 32 ビット
	start uint32 // replay_packet_id の上の 32 ビット。つないだときの時刻

	hmacSend, hmacRecv func() hash.Hash // tls-auth
	hmacLen            int
	keySend, keyRecv   []byte

	ke, ka, keIn, kaIn []byte // tls-crypt
	crypt              bool
}

func newWrapper(c *Config) (*wrapper, error) {
	w := &wrapper{start: uint32(time.Now().Unix())}
	switch {
	case c.TLSCrypt != nil:
		// クライアントは向きを逆にして使う。送るのは [1]、受けるのは [0]
		k := c.TLSCrypt
		w.crypt = true
		w.ke, w.ka = k[1].cipher[:32], k[1].hmac[:32]
		w.keIn, w.kaIn = k[0].cipher[:32], k[0].hmac[:32]
	case c.TLSAuth != nil:
		h := digests[c.Auth]
		w.hmacSend, w.hmacRecv, w.hmacLen = h, h, h().Size()
		out, in := 0, 0 // key-direction が無ければ両方に [0] を使う
		switch c.TLSAuthDir {
		case 0:
			out, in = 0, 1
		case 1:
			out, in = 1, 0
		}
		w.keySend, w.keyRecv = c.TLSAuth[out].hmac[:w.hmacLen], c.TLSAuth[in].hmac[:w.hmacLen]
	}
	return w, nil
}

func (w *wrapper) replayID() []byte {
	w.mu.Lock()
	w.next++
	n := w.next
	w.mu.Unlock()
	b := binary.BigEndian.AppendUint32(nil, n)
	return binary.BigEndian.AppendUint32(b, w.start)
}

// wrap は送る制御パケットを、回線に出す形にする
func (w *wrapper) wrap(p *ctrlPacket) []byte {
	head := append([]byte{p.op<<3 | p.keyID}, p.sid[:]...)
	body := p.body()
	switch {
	case w.crypt:
		rid := w.replayID()
		m := hmac.New(sha256.New, w.ka)
		m.Write(head)
		m.Write(rid)
		m.Write(body)
		tag := m.Sum(nil)
		enc := make([]byte, len(body))
		blk, _ := aes.NewCipher(w.ke)
		cipher.NewCTR(blk, tag[:16]).XORKeyStream(enc, body)
		out := append(head, rid...)
		out = append(out, tag...)
		return append(out, enc...)
	case w.hmacSend != nil:
		rid := w.replayID()
		m := hmac.New(w.hmacSend, w.keySend)
		m.Write(rid)
		m.Write(head)
		m.Write(body)
		out := append(head, m.Sum(nil)...)
		out = append(out, rid...)
		return append(out, body...)
	}
	return append(head, body...)
}

var errAuth = errors.New("制御チャネルのパケットの認証が合いません")

// unwrap は受けた制御パケットの包みを外して読む
func (w *wrapper) unwrap(b []byte) (*ctrlPacket, error) {
	if len(b) < 9 {
		return nil, errShort
	}
	p := &ctrlPacket{op: b[0] >> 3, keyID: b[0] & 7}
	copy(p.sid[:], b[1:9])
	head, rest := b[:9], b[9:]
	switch {
	case w.crypt:
		if len(rest) < 8+32 {
			return nil, errShort
		}
		rid, tag, enc := rest[:8], rest[8:40], rest[40:]
		body := make([]byte, len(enc))
		blk, _ := aes.NewCipher(w.keIn)
		cipher.NewCTR(blk, tag[:16]).XORKeyStream(body, enc)
		m := hmac.New(sha256.New, w.kaIn)
		m.Write(head)
		m.Write(rid)
		m.Write(body)
		if !hmac.Equal(m.Sum(nil), tag) {
			return nil, errAuth
		}
		rest = body
	case w.hmacRecv != nil:
		if len(rest) < w.hmacLen+8 {
			return nil, errShort
		}
		mac, rid, body := rest[:w.hmacLen], rest[w.hmacLen:w.hmacLen+8], rest[w.hmacLen+8:]
		m := hmac.New(w.hmacRecv, w.keyRecv)
		m.Write(rid)
		m.Write(head)
		m.Write(body)
		if !hmac.Equal(m.Sum(nil), mac) {
			return nil, errAuth
		}
		rest = body
	}
	return p, parseBody(p, rest)
}
