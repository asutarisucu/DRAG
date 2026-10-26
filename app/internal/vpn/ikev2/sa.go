package ikev2

import (
	"encoding/binary"
	"errors"
)

// ikeSA は IKE の SA の鍵。initiator はこの SA を始めたのが DRAG か。サーバーが鍵を替えると、始めた側が入れ替わる
type ikeSA struct {
	spiI, spiR [8]byte
	initiator  bool
	s          *suite
	d, pi, pr  []byte
	out, in    cipherKeys

	nextID     uint32 // DRAG が送る次の要求の番号
	nextPeerID uint32 // サーバーから来る次の要求の番号
	lastResp   []byte // 最後にサーバーに返した応答。同じ要求が来たら送り直す
	frags      map[uint16][]byte
	fragTotal  uint16
	fragNext   byte
}

// deriveIKE は SKEYSEED から IKE の SA の鍵を作る (RFC 7296 2.14)。ni と nr は SA を始めた側と応じた側のもの
func deriveIKE(s *suite, skeyseed, ni, nr []byte, spiI, spiR [8]byte, weInitiated bool) *ikeSA {
	pl, il, el := s.prfLen(), s.integKeyLen(), s.encrKeyLen()
	seed := append(append(append(append([]byte{}, ni...), nr...), spiI[:]...), spiR[:]...)
	km := s.prfPlus(skeyseed, seed, 3*pl+2*il+2*el)
	take := func(n int) []byte {
		b := km[:n]
		km = km[n:]
		return b
	}
	sa := &ikeSA{spiI: spiI, spiR: spiR, initiator: weInitiated, s: s}
	sa.d = take(pl)
	ai, ar := take(il), take(il)
	ei, er := take(el), take(el)
	sa.pi, sa.pr = take(pl), take(pl)
	i, r := newCipherKeys(s, ei, ai), newCipherKeys(s, er, ar)
	if weInitiated {
		sa.out, sa.in = i, r
	} else {
		sa.out, sa.in = r, i
	}
	return sa
}

func (sa *ikeSA) flags(response bool) byte {
	var f byte
	if sa.initiator {
		f |= flagInitiator
	}
	if response {
		f |= flagResponse
	}
	return f
}

// encrypt は、inner を SK に入れたメッセージを作る
func (sa *ikeSA) encrypt(m *message) ([]byte, error) {
	m.spiI, m.spiR = sa.spiI, sa.spiR
	inner := chain(m.inner, pNone)
	var plain []byte
	if sa.s.aead() {
		plain = append(inner, 0) // 詰め物なし
	} else {
		pad := (16 - (len(inner)+1)%16) % 16
		plain = append(inner, make([]byte, pad)...)
		plain = append(plain, byte(pad))
	}
	unenc := chain(m.payloads, pSK)
	skNext := first(m.inner, pNone)
	var head []byte
	body, err := sa.out.seal(func(encLen int) []byte {
		total := 28 + len(unenc) + 4 + encLen
		head = m.header(first(m.payloads, pSK), total)
		head = append(head, unenc...)
		head = append(head, skNext, 0)
		head = binary.BigEndian.AppendUint16(head, uint16(4+encLen))
		return head
	}, plain)
	if err != nil {
		return nil, err
	}
	return append(head, body...), nil
}

var errFragPending = errors.New("分けて届いたメッセージの残りを待っています")

// decrypt は受けたメッセージの SK を開き、inner に中のペイロードを入れる。
// 分けて届いたもの (SKF) は、全部そろうまで errFragPending を返す
func (sa *ikeSA) decrypt(raw []byte) (*message, error) {
	m, next, err := parseHeader(raw)
	if err != nil {
		return nil, err
	}
	ps, err := parsePayloads(next, raw[28:])
	if err != nil {
		return nil, err
	}
	if len(ps) == 0 || (ps[len(ps)-1].typ != pSK && ps[len(ps)-1].typ != pSKF) {
		return nil, errors.New("暗号にしていないメッセージは受けません")
	}
	last := ps[len(ps)-1]
	m.payloads = ps[:len(ps)-1]
	off := len(raw) - len(last.body)
	hl := 4
	if last.typ == pSKF {
		hl = 8
	}
	if len(last.body) < hl {
		return nil, errMalformed
	}
	plain, err := sa.in.open(raw[:off+hl], raw[off+hl:])
	if err != nil {
		return nil, err
	}
	if len(plain) < 1 || int(plain[len(plain)-1])+1 > len(plain) {
		return nil, errMalformed
	}
	content := plain[:len(plain)-1-int(plain[len(plain)-1])]
	firstType := last.body[0]
	if last.typ == pSKF {
		num, total := binary.BigEndian.Uint16(last.body[4:6]), binary.BigEndian.Uint16(last.body[6:8])
		if num == 0 || total == 0 || num > total {
			return nil, errMalformed
		}
		if sa.frags == nil || total > sa.fragTotal {
			sa.frags, sa.fragTotal = map[uint16][]byte{}, total
		}
		sa.frags[num] = content
		if num == 1 {
			sa.fragNext = firstType
		}
		if len(sa.frags) < int(sa.fragTotal) {
			return nil, errFragPending
		}
		content = nil
		for i := uint16(1); i <= sa.fragTotal; i++ {
			content = append(content, sa.frags[i]...)
		}
		firstType = sa.fragNext
		sa.frags = nil
	}
	if m.inner, err = parsePayloads(firstType, content); err != nil {
		return nil, err
	}
	return m, nil
}
