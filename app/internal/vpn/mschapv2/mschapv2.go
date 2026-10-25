// Package mschapv2 は MS-CHAPv2 (RFC 2759) の計算と、そこから作る MPPE の鍵 (RFC 3079)。
// IKEv2 の EAP-MSCHAPv2 と、SSTP や L2TP の PPP の認証で使う
package mschapv2

import (
	"bytes"
	"crypto/des"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"unicode/utf16"

	"golang.org/x/crypto/md4"
)

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

func hashHash(pass string) []byte {
	h := md4.New()
	h.Write(ntPasswordHash(pass))
	return h.Sum(nil)
}

func challengeHash(peer, auth []byte, user string) []byte {
	h := sha1.New()
	h.Write(peer)
	h.Write(auth)
	h.Write([]byte(userOnly(user)))
	return h.Sum(nil)[:8]
}

// NTResponse は、サーバーの challenge と自分の challenge から、24 バイトの NT-Response を作る
func NTResponse(auth, peer []byte, user, pass string) []byte {
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

// AuthenticatorResponse は、サーバーが Success で返すはずの "S=..." の文字列
func AuthenticatorResponse(pass string, ntResp, peer, auth []byte, user string) string {
	h := sha1.New()
	h.Write(hashHash(pass))
	h.Write(ntResp)
	h.Write(authMagic1)
	d := h.Sum(nil)
	h = sha1.New()
	h.Write(d)
	h.Write(challengeHash(peer, auth, user))
	h.Write(authMagic2)
	return "S=" + strings.ToUpper(hex.EncodeToString(h.Sum(nil)))
}

// CheckSuccess は、Success の文字列 ("S=... M=...") が正しいパスワードから作られたかを確かめる
func CheckSuccess(msg, pass string, ntResp, peer, auth []byte, user string) bool {
	return strings.HasPrefix(strings.ToUpper(msg), AuthenticatorResponse(pass, ntResp, peer, auth, user))
}

// RFC 3079 の MPPE の鍵の素
var (
	mppeMagic1 = []byte("This is the MPPE Master Key")
	mppeMagic2 = []byte("On the client side, this is the send key; on the server side, it is the receive key.")
	mppeMagic3 = []byte("On the client side, this is the receive key; on the server side, it is the send key.")
)

// ClientKeys は、クライアントの側の MasterSendKey と MasterReceiveKey (RFC 3079 3.4、どちらも 16 バイト)
func ClientKeys(pass string, ntResp []byte) (send, recv []byte) {
	h := sha1.New()
	h.Write(hashHash(pass))
	h.Write(ntResp)
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
	return key(mppeMagic2), key(mppeMagic3)
}
