package ikev2

import (
	"bytes"
	"crypto/des"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"

	"golang.org/x/crypto/md4"
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
		c.ntResponse = ntResponse(c.authChallenge, c.peerChallenge, c.user, c.pass)
		val := append(append(append([]byte{}, c.peerChallenge...), make([]byte, 8)...), c.ntResponse...)
		val = append(val, 0)
		body := []byte{msResponse, msID, 0, 0, 49}
		body = append(body, val...)
		body = append(body, c.user...)
		binary.BigEndian.PutUint16(body[2:4], uint16(len(body)))
		return eapPacket(eapResponse, id, eapMSCHAPv2, body), nil
	case msSuccess:
		msg := string(data[4:])
		want := authenticatorResponse(c.pass, c.ntResponse, c.peerChallenge, c.authChallenge, c.user)
		if !strings.HasPrefix(strings.ToUpper(msg), want) {
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

// userOnly は、DOMAIN\user のドメインを外した名前
func userOnly(user string) string {
	if i := strings.LastIndexByte(user, '\\'); i >= 0 {
		return user[i+1:]
	}
	return user
}

func ntPasswordHash(pass string) []byte {
	h := md4.New()
	for _, u := range utf16.Encode([]rune(pass)) {
		h.Write([]byte{byte(u), byte(u >> 8)})
	}
	return h.Sum(nil)
}

func challengeHash(peer, auth []byte, user string) []byte {
	h := sha1.New()
	h.Write(peer)
	h.Write(auth)
	h.Write([]byte(userOnly(user)))
	return h.Sum(nil)[:8]
}

func ntResponse(auth, peer []byte, user, pass string) []byte {
	ch := challengeHash(peer, auth, user)
	z := append(ntPasswordHash(pass), make([]byte, 5)...)
	var out []byte
	for i := 0; i < 3; i++ {
		out = append(out, desEncrypt(ch, z[i*7:i*7+7])...)
	}
	return out
}

// desEncrypt は 7 バイトの鍵を、パリティの位置を空けた 8 バイトにして DES で暗号にする
func desEncrypt(clear, key7 []byte) []byte {
	var k [8]byte
	var v uint64
	for _, b := range key7 {
		v = v<<8 | uint64(b)
	}
	for i := 0; i < 8; i++ {
		k[i] = byte(v>>(56-7*(i+1))) << 1
	}
	blk, _ := des.NewCipher(k[:])
	out := make([]byte, 8)
	blk.Encrypt(out, clear)
	return out
}

var (
	authMagic1 = []byte("Magic server to client signing constant")
	authMagic2 = []byte("Pad to make it do more than one iteration")
)

func authenticatorResponse(pass string, ntResp, peer, auth []byte, user string) string {
	hh := md4.New()
	hh.Write(ntPasswordHash(pass))
	h := sha1.New()
	h.Write(hh.Sum(nil))
	h.Write(ntResp)
	h.Write(authMagic1)
	d := h.Sum(nil)
	h = sha1.New()
	h.Write(d)
	h.Write(challengeHash(peer, auth, user))
	h.Write(authMagic2)
	return "S=" + strings.ToUpper(hex.EncodeToString(h.Sum(nil)))
}

// RFC 3079 の MPPE の鍵の素
var (
	mppeMagic1 = []byte("This is the MPPE Master Key")
	mppeMagic2 = []byte("On the client side, this is the send key; on the server side, it is the receive key.")
	mppeMagic3 = []byte("On the client side, this is the receive key; on the server side, it is the send key.")
)

// msk は IKEv2 の AUTH に使う鍵。サーバーと同じく、受ける鍵と送る鍵 (サーバーの側の呼び名) を並べ、0 で 64 バイトにする
func (c *mschap) msk() []byte {
	hh := md4.New()
	hh.Write(ntPasswordHash(c.pass))
	h := sha1.New()
	h.Write(hh.Sum(nil))
	h.Write(c.ntResponse)
	h.Write(mppeMagic1)
	master := h.Sum(nil)[:16]
	pad1, pad2 := make([]byte, 40), bytes.Repeat([]byte{0xf2}, 40)
	key := func(magic []byte) []byte {
		h := sha1.New()
		h.Write(master)
		h.Write(pad1)
		h.Write(magic)
		h.Write(pad2)
		return h.Sum(nil)[:16]
	}
	return append(append(key(mppeMagic2), key(mppeMagic3)...), make([]byte, 32)...)
}
