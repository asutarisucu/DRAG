package l2tp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"
)

// Phase 1 の暗号とハッシュ (RFC 2409 付録 A、RFC 3602、RFC 4868)
const (
	encr3DES  = 5
	encrAES   = 7
	hashSHA1  = 2
	hashSHA2  = 4
	authPSK   = 1
	esp3DES   = 3  // Phase 2 の ESP の変換 (RFC 2407 4.4.4)
	espAES    = 12 // RFC 3602
	authSHA1  = 2  // HMAC-SHA1-96
	authSHA2  = 5  // HMAC-SHA2-256-128 (RFC 4868)
	nonceSize = 32
)

// p1Suite は Phase 1 で決めた組
type p1Suite struct {
	encr, keyLen, hash, group uint32
}

// p1Suites は Phase 1 に出す提案。強いものから並べる
var p1Suites = []p1Suite{
	{encrAES, 256, hashSHA2, 14},
	{encrAES, 256, hashSHA1, 14},
	{encrAES, 128, hashSHA1, 14},
	{encrAES, 256, hashSHA1, 2},
	{encrAES, 128, hashSHA1, 2},
	{encr3DES, 0, hashSHA1, 2},
}

func (s p1Suite) attrs(lifetime uint32) []byte {
	b := basic(p1Encr, uint16(s.encr))
	if s.keyLen != 0 {
		b = append(b, basic(p1KeyLen, uint16(s.keyLen))...)
	}
	b = append(b, basic(p1Hash, uint16(s.hash))...)
	b = append(b, basic(p1Auth, authPSK)...)
	b = append(b, basic(p1Group, uint16(s.group))...)
	b = append(b, basic(p1LifeType, 1)...)
	return append(b, life(p1Life, lifetime)...)
}

// p2Suite は ESP の組
type p2Suite struct {
	encr, keyLen, auth uint32
}

var p2Suites = []p2Suite{
	{espAES, 256, authSHA1},
	{espAES, 128, authSHA1},
	{espAES, 256, authSHA2},
	{espAES, 128, authSHA2},
	{esp3DES, 0, authSHA1},
}

func (s p2Suite) attrs(lifetime uint32) []byte {
	b := basic(p2LifeType, 1)
	b = append(b, life(p2Life, lifetime)...)
	b = append(b, basic(p2Mode, modeUDPTransport)...)
	b = append(b, basic(p2Auth, uint16(s.auth))...)
	if s.keyLen != 0 {
		b = append(b, basic(p2KeyLen, uint16(s.keyLen))...)
	}
	return b
}

func hashOf(id uint32) func() hash.Hash {
	if id == hashSHA2 {
		return sha256.New
	}
	return sha1.New
}

func prf(h func() hash.Hash, key []byte, data ...[]byte) []byte {
	m := hmac.New(h, key)
	for _, d := range data {
		m.Write(d)
	}
	return m.Sum(nil)
}

// newBlock は CBC の暗号を作る。des3 なら 3DES、そうでなければ AES
func newBlock(des3 bool, key []byte) (cipher.Block, error) {
	if des3 {
		return des.NewTripleDESCipher(key)
	}
	return aes.NewCipher(key)
}

// isakmpCrypto は Phase 1 で決めた ISAKMP の SA の鍵
type isakmpCrypto struct {
	h       func() hash.Hash
	block   cipher.Block
	skeyid  []byte
	d, a, e []byte // SKEYID_d、SKEYID_a、SKEYID_e
	lastIV  []byte // Phase 1 の最後の CBC の出力。Quick Mode と Informational の IV のもと
}

// derive は SKEYID とその先の鍵を作る (RFC 2409 5、付録 B)
func derive(s p1Suite, psk, ni, nr, gxy []byte, ispi, rspi [8]byte) (*isakmpCrypto, error) {
	h := hashOf(s.hash)
	k := &isakmpCrypto{h: h}
	k.skeyid = prf(h, psk, ni, nr)
	k.d = prf(h, k.skeyid, gxy, ispi[:], rspi[:], []byte{0})
	k.a = prf(h, k.skeyid, k.d, gxy, ispi[:], rspi[:], []byte{1})
	k.e = prf(h, k.skeyid, k.a, gxy, ispi[:], rspi[:], []byte{2})
	n := 24
	if s.encr == encrAES {
		n = int(s.keyLen) / 8
	}
	// SKEYID_e が足りなければ K1 = prf(SKEYID_e, 0)、K2 = prf(SKEYID_e, K1) とつなぐ
	key := k.e
	if len(key) < n {
		key = nil
		for kn := []byte{0}; len(key) < n; {
			kn = prf(h, k.e, kn)
			key = append(key, kn...)
		}
	}
	var err error
	k.block, err = newBlock(s.encr == encr3DES, key[:n])
	return k, err
}

// encrypt は ISAKMP のメッセージの本体を CBC で暗号にする。詰め物は 0 で、最後のブロックを次の IV として返す
func (k *isakmpCrypto) encrypt(iv, plain []byte) (ct, next []byte) {
	bs := k.block.BlockSize()
	if r := len(plain) % bs; r != 0 {
		plain = append(plain, make([]byte, bs-r)...)
	}
	ct = make([]byte, len(plain))
	cipher.NewCBCEncrypter(k.block, iv).CryptBlocks(ct, plain)
	return ct, ct[len(ct)-bs:]
}

func (k *isakmpCrypto) decrypt(iv, ct []byte) (plain, next []byte, err error) {
	bs := k.block.BlockSize()
	if len(ct) == 0 || len(ct)%bs != 0 {
		return nil, nil, errMalformed
	}
	plain = make([]byte, len(ct))
	cipher.NewCBCDecrypter(k.block, iv).CryptBlocks(plain, ct)
	return plain, append([]byte{}, ct[len(ct)-bs:]...), nil
}

// phase2IV は Quick Mode と Informational の最初の IV (RFC 2409 付録 B)
func (k *isakmpCrypto) phase2IV(msgID uint32) []byte {
	h := k.h()
	h.Write(k.lastIV)
	h.Write(binary.BigEndian.AppendUint32(nil, msgID))
	return h.Sum(nil)[:k.block.BlockSize()]
}

// keymat は ESP の SA の鍵 (RFC 2409 5.5)。KEYMAT = K1 | K2 | …、Kn = prf(SKEYID_d, Kn-1 | protocol | SPI | Ni_b | Nr_b)
func (k *isakmpCrypto) keymat(spi uint32, ni, nr []byte, n int) []byte {
	seed := append([]byte{protoESP}, binary.BigEndian.AppendUint32(nil, spi)...)
	seed = append(append(seed, ni...), nr...)
	var out, kn []byte
	for len(out) < n {
		kn = prf(k.h, k.d, kn, seed)
		out = append(out, kn...)
	}
	return out[:n]
}

var errHash = errors.New("VPN のサーバーの HASH が合いません")
