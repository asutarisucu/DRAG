// Package ikev2 は IKEv2 (RFC 7296) のクライアントと、UDP に包んだ ESP (RFC 3948)。
// サーバーとは最初から 4500 番の形で話し、ESP も UDP に包ませる。
// 利用者の認証は EAP-MSCHAPv2、サーバーは証明書で確かめる
package ikev2

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// 交換の種類 (RFC 7296 3.1)
const (
	exInit        = 34
	exAuth        = 35
	exCreateChild = 36
	exInfo        = 37
)

// IKE のヘッダーの flags
const (
	flagInitiator = 0x08
	flagResponse  = 0x20
)

// ペイロードの種類 (RFC 7296 3.2、RFC 7383)
const (
	pNone    = 0
	pSA      = 33
	pKE      = 34
	pIDi     = 35
	pIDr     = 36
	pCert    = 37
	pCertReq = 38
	pAuth    = 39
	pNonce   = 40
	pNotify  = 41
	pDelete  = 42
	pTSi     = 44
	pTSr     = 45
	pSK      = 46
	pCP      = 47
	pEAP     = 48
	pSKF     = 53
)

// Notify の種類 (RFC 7296 3.10.1、RFC 7383、RFC 7427)
const (
	nAuthFailed      = 24
	nNoProposal      = 14
	nInvalidKE       = 17
	nTSUnacceptable  = 38
	nInternalAddress = 36
	nInitialContact  = 16384
	nNATSource       = 16388
	nNATDest         = 16389
	nCookie          = 16390
	nRekeySA         = 16393
	nFragSupported   = 16430
	nSigHashAlgs     = 16431
)

// プロトコル (RFC 7296 3.3.1)
const (
	protoIKE = 1
	protoESP = 3
)

type payload struct {
	typ  byte
	body []byte // 汎用のヘッダー (4 バイト) のあと
}

// message は IKE のメッセージ1つ。SK の中のペイロードは inner に入れる
type message struct {
	spiI, spiR [8]byte
	exch       byte
	flags      byte
	id         uint32
	payloads   []payload // 暗号にしない部分
	inner      []payload // SK の中
}

func (m *message) isResponse() bool { return m.flags&flagResponse != 0 }

func (m *message) header(next byte, length int) []byte {
	b := append(append([]byte{}, m.spiI[:]...), m.spiR[:]...)
	b = append(b, next, 0x20, m.exch, m.flags)
	b = binary.BigEndian.AppendUint32(b, m.id)
	return binary.BigEndian.AppendUint32(b, uint32(length))
}

// chain はペイロードを、次の種類をつないで並べる。最後の次は last にする
func chain(ps []payload, last byte) []byte {
	var b []byte
	for i, p := range ps {
		next := last
		if i+1 < len(ps) {
			next = ps[i+1].typ
		}
		b = append(b, next, 0)
		b = binary.BigEndian.AppendUint16(b, uint16(4+len(p.body)))
		b = append(b, p.body...)
	}
	return b
}

func first(ps []payload, def byte) byte {
	if len(ps) == 0 {
		return def
	}
	return ps[0].typ
}

// encodePlain は暗号にしないメッセージ (IKE_SA_INIT) を作る
func (m *message) encodePlain() []byte {
	body := chain(m.payloads, pNone)
	return append(m.header(first(m.payloads, pNone), 28+len(body)), body...)
}

var errMalformed = errors.New("IKE のメッセージを読めません")

// parsePayloads はペイロードの並びを読む。SK を見つけたら、そこで止めて返す
func parsePayloads(next byte, b []byte) ([]payload, error) {
	var out []payload
	for next != pNone {
		if len(b) < 4 {
			return nil, errMalformed
		}
		n := int(binary.BigEndian.Uint16(b[2:4]))
		if n < 4 || n > len(b) {
			return nil, errMalformed
		}
		out = append(out, payload{typ: next, body: b[4:n]})
		if next == pSK || next == pSKF {
			// SK と SKF の次の種類は、中の最初のペイロード
			out[len(out)-1].body = b[:n]
			return out, nil
		}
		next = b[0]
		b = b[n:]
	}
	return out, nil
}

// parseHeader はヘッダーを読む。ペイロードは読まない
func parseHeader(b []byte) (*message, byte, error) {
	if len(b) < 28 {
		return nil, 0, errMalformed
	}
	m := &message{exch: b[18], flags: b[19], id: binary.BigEndian.Uint32(b[20:24])}
	copy(m.spiI[:], b[0:8])
	copy(m.spiR[:], b[8:16])
	if b[17]>>4 != 2 {
		return nil, 0, fmt.Errorf("IKE の版 %d には対応していません", b[17]>>4)
	}
	if int(binary.BigEndian.Uint32(b[24:28])) != len(b) {
		return nil, 0, errMalformed
	}
	return m, b[16], nil
}

func find(ps []payload, typ byte) []byte {
	for _, p := range ps {
		if p.typ == typ {
			return p.body
		}
	}
	return nil
}

// notify は Notify のペイロード。SPI は空
func notify(typ uint16, data []byte) payload {
	b := []byte{0, 0}
	b = binary.BigEndian.AppendUint16(b, typ)
	return payload{typ: pNotify, body: append(b, data...)}
}

type notification struct {
	proto byte
	typ   uint16
	spi   []byte
	data  []byte
}

func notifications(ps []payload) []notification {
	var out []notification
	for _, p := range ps {
		if p.typ != pNotify || len(p.body) < 4 {
			continue
		}
		n := int(p.body[1])
		if len(p.body) < 4+n {
			continue
		}
		out = append(out, notification{proto: p.body[0], typ: binary.BigEndian.Uint16(p.body[2:4]), spi: p.body[4 : 4+n], data: p.body[4+n:]})
	}
	return out
}

// errorNotify は、応答の中のエラーの Notify (16383 以下) を返す
func errorNotify(ps []payload) *notification {
	for _, n := range notifications(ps) {
		if n.typ < 16384 {
			return &n
		}
	}
	return nil
}

var notifyNames = map[uint16]string{
	nAuthFailed: "認証に失敗しました (AUTHENTICATION_FAILED)", nNoProposal: "使える暗号の組がありません (NO_PROPOSAL_CHOSEN)",
	nTSUnacceptable: "通す範囲が受け付けられません (TS_UNACCEPTABLE)", nInternalAddress: "VPN の中のアドレスをもらえません (INTERNAL_ADDRESS_FAILURE)",
	nInvalidKE: "鍵の交換の方式が違います (INVALID_KE_PAYLOAD)",
}

func (n *notification) Error() string {
	if s, ok := notifyNames[n.typ]; ok {
		return "VPN のサーバー: " + s
	}
	return fmt.Sprintf("VPN のサーバーがエラー %d を返しました", n.typ)
}
