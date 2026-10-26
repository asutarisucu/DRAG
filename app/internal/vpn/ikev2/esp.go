package ikev2

import (
	"crypto/aes"
	"encoding/binary"
	"errors"
	"math"
	"sync"
)

// childSA は ESP の SA の組 (RFC 4303)。トンネルモードで、IP のパケットを丸ごと包む
type childSA struct {
	inSPI, outSPI uint32
	in, out       cipherKeys
	prop          proposal // サーバーが選んだ提案
	gen           int      // 作った順の番号

	mu     sync.Mutex
	seq    uint32
	replay replayWindow
}

// newChildSA は KEYMAT から鍵を取る。交換を始めた側から応じた側への鍵が前にある (RFC 7296 2.17)
func newChildSA(s *suite, keymat []byte, inSPI, outSPI uint32, weInitiated bool, prop proposal) *childSA {
	el, il := s.encrKeyLen(), s.integKeyLen()
	take := func() cipherKeys {
		k := newCipherKeys(s, keymat[:el], keymat[el:el+il])
		keymat = keymat[el+il:]
		return k
	}
	first, second := take(), take()
	c := &childSA{inSPI: inSPI, outSPI: outSPI, prop: prop}
	if weInitiated {
		c.out, c.in = first, second
	} else {
		c.in, c.out = first, second
	}
	return c
}

func childKeyLen(s *suite) int { return 2 * (s.encrKeyLen() + s.integKeyLen()) }

// seal は IP のパケットを ESP にする
func (c *childSA) seal(ip []byte) ([]byte, error) {
	c.mu.Lock()
	if c.seq == math.MaxUint32 {
		// 拡張した番号 (ESN) を使わないので、一周させずに止める (RFC 4303 3.3.3)。ふつうはその前にサーバーが鍵を替える
		c.mu.Unlock()
		return nil, errors.New("ESP の SA の番号を使い切りました")
	}
	c.seq++
	seq := c.seq
	c.mu.Unlock()
	next := byte(4)
	if len(ip) > 0 && ip[0]>>4 == 6 {
		next = 41
	}
	block := 4
	if !c.out.s.aead() {
		block = aes.BlockSize
	}
	pad := (block - (len(ip)+2)%block) % block
	plain := append([]byte{}, ip...)
	for i := 1; i <= pad; i++ {
		plain = append(plain, byte(i))
	}
	plain = append(plain, byte(pad), next)
	head := binary.BigEndian.AppendUint32(nil, c.outSPI)
	head = binary.BigEndian.AppendUint32(head, seq)
	body, err := c.out.seal(func(int) []byte { return head }, plain)
	if err != nil {
		return nil, err
	}
	return append(head, body...), nil
}

var errESPReplay = errors.New("同じ ESP のパケットをもう一度受けました")

// open は ESP を開き、中の IP のパケットを返す。中身の無いパケット (次のヘッダーが 59) は nil を返す
func (c *childSA) open(b []byte) ([]byte, error) {
	if len(b) < 8 {
		return nil, errMalformed
	}
	plain, err := c.in.open(b[:8], b[8:])
	if err != nil {
		return nil, err
	}
	if len(plain) < 2 {
		return nil, errMalformed
	}
	c.mu.Lock()
	ok := c.replay.accept(binary.BigEndian.Uint32(b[4:8]))
	c.mu.Unlock()
	if !ok {
		return nil, errESPReplay
	}
	pad, next := int(plain[len(plain)-2]), plain[len(plain)-1]
	if pad+2 > len(plain) {
		return nil, errMalformed
	}
	if next == 59 {
		return nil, nil
	}
	return plain[:len(plain)-2-pad], nil
}

// replayWindow は、受けた番号のうち、いちばん大きいものとその手前 64 個を覚える
type replayWindow struct {
	top  uint32
	seen uint64
}

func (w *replayWindow) accept(id uint32) bool {
	switch {
	case id == 0:
		return false
	case id > w.top:
		d := id - w.top
		if d >= 64 {
			w.seen = 0
		} else {
			w.seen <<= d
		}
		w.seen |= 1
		w.top = id
		return true
	case w.top-id >= 64:
		return false
	default:
		bit := uint64(1) << (w.top - id)
		if w.seen&bit != 0 {
			return false
		}
		w.seen |= bit
		return true
	}
}
