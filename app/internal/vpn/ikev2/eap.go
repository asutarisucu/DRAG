package ikev2

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/asutarisucu/DRAG/app/internal/vpn/mschapv2"
)

// EAP (RFC 3748) と EAP-MSCHAPv2 (RFC 2759、draft-kamath-pppext-eap-mschapv2)
const (
	eapRequest  = 1
	eapResponse = 2
	eapSuccess  = 3
	eapFailure  = 4

	eapIdentity = 1
	eapNak      = 3
	eapMSCHAPv2 = 26

	msChallenge = 1
	msResponse  = 2
	msSuccess   = 3
	msFailure   = 4
)

// mschap は EAP-MSCHAPv2 の利用者の側
type mschap struct {
	user, pass    string
	authChallenge []byte
	peerChallenge []byte
	ntResponse    []byte
	verified      bool // サーバーが正しいパスワードを知っていると確かめた
}

var errEAPFailed = errors.New("VPN のサーバーがユーザー名かパスワードを受け付けません")

// respond は EAP の要求に返す。nil を返したら返すものは無い
func (c *mschap) respond(req []byte) ([]byte, error) {
	if len(req) < 4 {
		return nil, errMalformed
	}
	code, id := req[0], req[1]
	switch code {
	case eapSuccess:
		if !c.verified {
			return nil, errors.New("VPN のサーバーがパスワードを確かめずに通しました")
		}
		return nil, nil
	case eapFailure:
		return nil, errEAPFailed
	case eapRequest:
	default:
		return nil, errMalformed
	}
	if len(req) < 5 {
		return nil, errMalformed
	}
	typ, data := req[4], req[5:]
	switch typ {
	case eapIdentity:
		return eapPacket(eapResponse, id, eapIdentity, []byte(c.user)), nil
	case eapMSCHAPv2:
	default:
		// ほかの方式を求められたら、MSCHAPv2 にしてほしいと返す
		return eapPacket(eapResponse, id, eapNak, []byte{eapMSCHAPv2}), nil
	}
	if len(data) < 4 {
		return nil, errMalformed
	}
	op, msID := data[0], data[1]
	switch op {
	case msChallenge:
		if len(data) < 5+16 || data[4] != 16 {
			return nil, errMalformed
		}
		c.authChallenge = append([]byte{}, data[5:21]...)
		c.peerChallenge = make([]byte, 16)
		rand.Read(c.peerChallenge)
		c.ntResponse = mschapv2.NTResponse(c.authChallenge, c.peerChallenge, c.user, c.pass)
		val := append(append(append([]byte{}, c.peerChallenge...), make([]byte, 8)...), c.ntResponse...)
		val = append(val, 0)
		body := []byte{msResponse, msID, 0, 0, 49}
		body = append(body, val...)
		body = append(body, c.user...)
		binary.BigEndian.PutUint16(body[2:4], uint16(len(body)))
		return eapPacket(eapResponse, id, eapMSCHAPv2, body), nil
	case msSuccess:
		if !mschapv2.CheckSuccess(string(data[4:]), c.pass, c.ntResponse, c.peerChallenge, c.authChallenge, c.user) {
			return nil, errors.New("VPN のサーバーの応答が合いません。パスワードを知らないサーバーかもしれません")
		}
		c.verified = true
		return eapPacket(eapResponse, id, eapMSCHAPv2, []byte{msSuccess}), nil
	case msFailure:
		msg := string(data[4:])
		if strings.Contains(msg, "E=648") {
			return nil, errors.New("VPN のパスワードの期限が切れています")
		}
		return nil, fmt.Errorf("%w (%s)", errEAPFailed, msg)
	}
	return nil, errMalformed
}

func eapPacket(code, id, typ byte, data []byte) []byte {
	b := []byte{code, id, 0, 0, typ}
	b = append(b, data...)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(b)))
	return b
}

// msk は IKEv2 の AUTH に使う鍵。MasterSendKey と MasterReceiveKey を並べ、0 で 64 バイトにする。
// strongSwan の作り方と同じ (呼び名はサーバーの側から見て逆になっている)
func (c *mschap) msk() []byte {
	send, recv := mschapv2.ClientKeys(c.pass, c.ntResponse)
	return append(append(send, recv...), make([]byte, 32)...)
}
