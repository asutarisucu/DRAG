package l2tp

import (
	"encoding/binary"
	"errors"
)

// ISAKMP (RFC 2408)、IKEv1 (RFC 2409)、IPsec DOI (RFC 2407)、NAT-T (RFC 3947) の番号
const (
	plSA        = 1
	plProposal  = 2
	plTransform = 3
	plKE        = 4
	plID        = 5
	plHash      = 8
	plNonce     = 10
	plNotify    = 11
	plDelete    = 12
	plVendor    = 13
	plNATD      = 20
	plNATOA     = 21

	exMain  = 2
	exInfo  = 5
	exQuick = 32

	flagEnc = 1

	doiIPsec    = 1
	protoISAKMP = 1
	protoESP    = 3
	idIPv4      = 1
	keyIKE      = 1 // Phase 1 の変換の番号 KEY_IKE

	// Phase 1 の属性 (RFC 2409 付録 A)
	p1Encr     = 1
	p1Hash     = 2
	p1Auth     = 3
	p1Group    = 4
	p1LifeType = 11
	p1Life     = 12
	p1KeyLen   = 14

	// Phase 2 の属性 (RFC 2407 4.5)
	p2LifeType = 1
	p2Life     = 2
	p2Mode     = 4
	p2Auth     = 5
	p2KeyLen   = 6

	modeUDPTransport = 4 // UDP-Encapsulated-Transport (RFC 3947)

	notifyRUThere    = 36136 // DPD (RFC 3706)
	notifyRUThereAck = 36137
)

var errMalformed = errors.New("IKE のメッセージを読めません")

type payload struct {
	typ  byte
	body []byte
}

// chain は payload を順につなぎ、それぞれの頭に次の payload の種類と長さを書く
func chain(ps ...payload) []byte {
	var b []byte
	for i, p := range ps {
		next := byte(0)
		if i+1 < len(ps) {
			next = ps[i+1].typ
		}
		b = append(b, next, 0)
		b = binary.BigEndian.AppendUint16(b, uint16(4+len(p.body)))
		b = append(b, p.body...)
	}
	return b
}

type header struct {
	ispi, rspi [8]byte
	next       byte
	exch       byte
	flags      byte
	msgID      uint32
}

func (h header) encode(bodyLen int) []byte {
	b := make([]byte, 28)
	copy(b, h.ispi[:])
	copy(b[8:], h.rspi[:])
	b[16], b[17], b[18], b[19] = h.next, 0x10, h.exch, h.flags
	binary.BigEndian.PutUint32(b[20:], h.msgID)
	binary.BigEndian.PutUint32(b[24:], uint32(28+bodyLen))
	return b
}

func parseHeader(b []byte) (header, []byte, error) {
	if len(b) < 28 {
		return header{}, nil, errMalformed
	}
	var h header
	copy(h.ispi[:], b[:8])
	copy(h.rspi[:], b[8:16])
	h.next, h.exch, h.flags = b[16], b[18], b[19]
	h.msgID = binary.BigEndian.Uint32(b[20:24])
	n := int(binary.BigEndian.Uint32(b[24:28]))
	if n < 28 || n > len(b) {
		return header{}, nil, errMalformed
	}
	return h, b[28:n], nil
}

// parsed は読んだ payload と、元のバイト列の中で頭を含めて占めていた範囲
type parsed struct {
	payload
	start, end int
}

// parse は first から始まる payload の列を読む。暗号のために足した詰め物は読まない
func parse(first byte, b []byte) ([]parsed, error) {
	var ps []parsed
	off := 0
	for next := first; next != 0; {
		if off+4 > len(b) {
			return nil, errMalformed
		}
		n := int(binary.BigEndian.Uint16(b[off+2 : off+4]))
		if n < 4 || off+n > len(b) {
			return nil, errMalformed
		}
		ps = append(ps, parsed{payload{next, b[off+4 : off+n]}, off, off + n})
		next = b[off]
		off += n
	}
	return ps, nil
}

func find(ps []parsed, typ byte) []byte {
	for _, p := range ps {
		if p.typ == typ {
			return p.body
		}
	}
	return nil
}

func findAll(ps []parsed, typ byte) [][]byte {
	var out [][]byte
	for _, p := range ps {
		if p.typ == typ {
			out = append(out, p.body)
		}
	}
	return out
}

// basic は 2 バイトに収まる属性。life は 4 バイトの可変長で書く
func basic(typ, v uint16) []byte {
	return binary.BigEndian.AppendUint16(binary.BigEndian.AppendUint16(nil, 0x8000|typ), v)
}

func life(typ uint16, sec uint32) []byte {
	b := binary.BigEndian.AppendUint16(nil, typ)
	b = binary.BigEndian.AppendUint16(b, 4)
	return binary.BigEndian.AppendUint32(b, sec)
}

// transform は SA の変換1つ。attrs は属性をつないだもの。life は秒で決めた寿命
type transform struct {
	id    byte
	attrs []byte
	vals  map[uint16]uint32
	life  uint32
}

type proposal struct {
	proto      byte
	spi        []byte
	transforms []transform
}

// encodeSA は提案1つの SA の payload の中身を作る (RFC 2408 3.4〜3.6)
func encodeSA(p proposal) []byte {
	b := binary.BigEndian.AppendUint32(nil, doiIPsec)
	b = binary.BigEndian.AppendUint32(b, 1) // SIT_IDENTITY_ONLY
	prop := append([]byte{1, p.proto, byte(len(p.spi)), byte(len(p.transforms))}, p.spi...)
	var ts []payload
	for i, t := range p.transforms {
		ts = append(ts, payload{plTransform, append([]byte{byte(i + 1), t.id, 0, 0}, t.attrs...)})
	}
	prop = append(prop, chain(ts...)...)
	return append(b, chain(payload{plProposal, prop})...)
}

// parseSA は SA の payload の中の提案を読む
func parseSA(b []byte) ([]proposal, error) {
	if len(b) < 8 || binary.BigEndian.Uint32(b) != doiIPsec {
		return nil, errMalformed
	}
	pps, err := parse(plProposal, b[8:])
	if err != nil {
		return nil, err
	}
	var out []proposal
	for _, pp := range pps {
		pb := pp.body
		if len(pb) < 4 || len(pb) < 4+int(pb[2]) {
			return nil, errMalformed
		}
		p := proposal{proto: pb[1], spi: pb[4 : 4+int(pb[2])]}
		tps, err := parse(plTransform, pb[4+int(pb[2]):])
		if err != nil {
			return nil, err
		}
		// 寿命は「種類」と「長さ」の2つの属性で決める。秒と KB の両方が来ることがある
		lifeType, lifeLen := uint16(p2LifeType), uint16(p2Life)
		if p.proto == protoISAKMP {
			lifeType, lifeLen = p1LifeType, p1Life
		}
		for _, tp := range tps {
			if len(tp.body) < 4 {
				return nil, errMalformed
			}
			t := transform{id: tp.body[1], attrs: tp.body[4:], vals: map[uint16]uint32{}}
			seconds := false
			for a := t.attrs; len(a) >= 4; {
				typ := binary.BigEndian.Uint16(a)
				var v uint32
				if typ&0x8000 != 0 {
					typ &= 0x7fff
					v = uint32(binary.BigEndian.Uint16(a[2:]))
					a = a[4:]
				} else {
					n := int(binary.BigEndian.Uint16(a[2:]))
					if len(a) < 4+n {
						return nil, errMalformed
					}
					for _, x := range a[4 : 4+n] {
						v = v<<8 | uint32(x)
					}
					a = a[4+n:]
				}
				t.vals[typ] = v
				switch typ {
				case lifeType:
					seconds = v == 1
				case lifeLen:
					if seconds {
						t.life = v
					}
				}
			}
			p.transforms = append(p.transforms, t)
		}
		out = append(out, p)
	}
	return out, nil
}

// idPayload は IPv4 のアドレスの ID (RFC 2407 4.6.2)
func idPayload(ip [4]byte, proto byte, port uint16) []byte {
	b := []byte{idIPv4, proto}
	b = binary.BigEndian.AppendUint16(b, port)
	return append(b, ip[:]...)
}

// natOA は NAT-OA の payload の中身 (RFC 3947 5.2)
func natOA(ip [4]byte) []byte { return append([]byte{idIPv4, 0, 0, 0}, ip[:]...) }
