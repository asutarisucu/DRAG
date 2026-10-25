package l2tp

import (
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"hash"
	"sync"
	"time"
)

// espSA は片方向の ESP の SA (RFC 4303)。トランスポートモードで、UDP (L2TP) を包む
type espSA struct {
	spi     uint32
	block   cipher.Block
	h       func() hash.Hash
	authKey []byte
	icvLen  int
	life    time.Duration
	expires time.Time

	mu     sync.Mutex
	seq    uint32
	top    uint32 // 受けた中でいちばん大きい番号
	window uint64 // top から下 64 個を受けたか
}

// newESP は KEYMAT から鍵を取る。暗号の鍵が先で、認証の鍵があと
func newESP(s p2Suite, spi uint32, keymat []byte, lifetime time.Duration) (*espSA, error) {
	el := 24
	if s.encr == espAES {
		el = int(s.keyLen) / 8
	}
	block, err := newBlock(s.encr == esp3DES, keymat[:el])
	if err != nil {
		return nil, err
	}
	e := &espSA{spi: spi, block: block, life: lifetime, expires: time.Now().Add(lifetime)}
	if s.auth == authSHA2 {
		e.h, e.authKey, e.icvLen = hashOf(hashSHA2), keymat[el:el+32], 16
	} else {
		e.h, e.authKey, e.icvLen = hashOf(hashSHA1), keymat[el:el+20], 12
	}
	return e, nil
}

func espKeyLen(s p2Suite) int {
	n := 24
	if s.encr == espAES {
		n = int(s.keyLen) / 8
	}
	if s.auth == authSHA2 {
		return n + 32
	}
	return n + 20
}

// seal は中身を ESP にする。next は中身のプロトコル (UDP なら 17)
func (e *espSA) seal(next byte, data []byte) []byte {
	e.mu.Lock()
	e.seq++
	seq := e.seq
	e.mu.Unlock()
	bs := e.block.BlockSize()
	pad := (bs - (len(data)+2)%bs) % bs
	plain := append([]byte{}, data...)
	for i := 1; i <= pad; i++ {
		plain = append(plain, byte(i))
	}
	plain = append(plain, byte(pad), next)
	b := binary.BigEndian.AppendUint32(nil, e.spi)
	b = binary.BigEndian.AppendUint32(b, seq)
	iv := make([]byte, bs)
	rand.Read(iv)
	b = append(b, iv...)
	ct := make([]byte, len(plain))
	cipher.NewCBCEncrypter(e.block, iv).CryptBlocks(ct, plain)
	b = append(b, ct...)
	m := hmac.New(e.h, e.authKey)
	m.Write(b)
	return append(b, m.Sum(nil)[:e.icvLen]...)
}

var errESP = errors.New("ESP のパケットを開けません")

// open は ESP を開き、中身のプロトコルと中身を返す
func (e *espSA) open(b []byte) (byte, []byte, error) {
	bs := e.block.BlockSize()
	if len(b) < 8+bs+bs+e.icvLen || (len(b)-8-bs-e.icvLen)%bs != 0 {
		return 0, nil, errESP
	}
	body, icv := b[:len(b)-e.icvLen], b[len(b)-e.icvLen:]
	m := hmac.New(e.h, e.authKey)
	m.Write(body)
	if !hmac.Equal(m.Sum(nil)[:e.icvLen], icv) {
		return 0, nil, errESP
	}
	seq := binary.BigEndian.Uint32(b[4:8])
	if !e.accept(seq) {
		return 0, nil, errESP
	}
	plain := make([]byte, len(body)-8-bs)
	cipher.NewCBCDecrypter(e.block, body[8:8+bs]).CryptBlocks(plain, body[8+bs:])
	pad := int(plain[len(plain)-2])
	if pad+2 > len(plain) {
		return 0, nil, errESP
	}
	return plain[len(plain)-1], plain[:len(plain)-2-pad], nil
}

// accept は、同じ番号のパケットを二度受けないようにする (RFC 4303 3.4.3)
func (e *espSA) accept(seq uint32) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case seq == 0:
		return false
	case seq > e.top:
		if d := seq - e.top; d < 64 {
			e.window = e.window<<d | 1
		} else {
			e.window = 1
		}
		e.top = seq
		return true
	case e.top-seq >= 64:
		return false
	default:
		bit := uint64(1) << (e.top - seq)
		if e.window&bit != 0 {
			return false
		}
		e.window |= bit
		return true
	}
}
