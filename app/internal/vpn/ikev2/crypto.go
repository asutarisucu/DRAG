package ikev2

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"math/big"

	"golang.org/x/crypto/chacha20poly1305"
)

// 変換の種類と番号 (RFC 7296 3.3.2、RFC 4868、RFC 5282、RFC 5903、RFC 7634、RFC 8031)
const (
	tEncr  = 1
	tPRF   = 2
	tInteg = 3
	tDH    = 4
	tESN   = 5

	encrAESCBC   = 12
	encrAESGCM16 = 20
	encrChaCha   = 28

	prfSHA1   = 2
	prfSHA256 = 5
	prfSHA384 = 6
	prfSHA512 = 7

	integSHA1   = 2  // HMAC-SHA1-96
	integSHA256 = 12 // HMAC-SHA2-256-128
	integSHA384 = 13 // HMAC-SHA2-384-192
	integSHA512 = 14 // HMAC-SHA2-512-256

	attrKeyLen = 14
)

type transform struct {
	typ    byte
	id     uint16
	keyLen int // ビット。鍵の長さを決める暗号だけ
}

// proposal は SA の提案1つ
type proposal struct {
	num        byte
	proto      byte
	spi        []byte
	transforms []transform
}

func encodeSA(props []proposal) []byte {
	var b []byte
	for i, p := range props {
		var tb []byte
		for j, t := range p.transforms {
			last := byte(3)
			if j == len(p.transforms)-1 {
				last = 0
			}
			body := []byte{t.typ, 0}
			body = binary.BigEndian.AppendUint16(body, t.id)
			if t.keyLen > 0 {
				body = binary.BigEndian.AppendUint16(body, 0x8000|attrKeyLen)
				body = binary.BigEndian.AppendUint16(body, uint16(t.keyLen))
			}
			tb = append(tb, last, 0)
			tb = binary.BigEndian.AppendUint16(tb, uint16(4+len(body)))
			tb = append(tb, body...)
		}
		last := byte(2)
		if i == len(props)-1 {
			last = 0
		}
		pb := []byte{p.num, p.proto, byte(len(p.spi)), byte(len(p.transforms))}
		pb = append(pb, p.spi...)
		pb = append(pb, tb...)
		b = append(b, last, 0)
		b = binary.BigEndian.AppendUint16(b, uint16(4+len(pb)))
		b = append(b, pb...)
	}
	return b
}

func parseSA(b []byte) ([]proposal, error) {
	var out []proposal
	for len(b) > 0 {
		if len(b) < 8 {
			return nil, errMalformed
		}
		n := int(binary.BigEndian.Uint16(b[2:4]))
		if n < 8 || n > len(b) {
			return nil, errMalformed
		}
		pb := b[4:n]
		p := proposal{num: pb[0], proto: pb[1]}
		spiLen, nt := int(pb[2]), int(pb[3])
		if len(pb) < 4+spiLen {
			return nil, errMalformed
		}
		p.spi = pb[4 : 4+spiLen]
		tb := pb[4+spiLen:]
		for i := 0; i < nt; i++ {
			if len(tb) < 8 {
				return nil, errMalformed
			}
			tn := int(binary.BigEndian.Uint16(tb[2:4]))
			if tn < 8 || tn > len(tb) {
				return nil, errMalformed
			}
			t := transform{typ: tb[4], id: binary.BigEndian.Uint16(tb[6:8])}
			for a := tb[8:tn]; len(a) >= 4; {
				at := binary.BigEndian.Uint16(a[0:2])
				if at&0x8000 != 0 {
					if at&0x7fff == attrKeyLen {
						t.keyLen = int(binary.BigEndian.Uint16(a[2:4]))
					}
					a = a[4:]
					continue
				}
				l := int(binary.BigEndian.Uint16(a[2:4]))
				if len(a) < 4+l {
					break
				}
				a = a[4+l:]
			}
			p.transforms = append(p.transforms, t)
			tb = tb[tn:]
		}
		out = append(out, p)
		b = b[n:]
	}
	return out, nil
}

// get は、その種類の変換を返す。無ければ ok が false
func (p *proposal) get(typ byte) (transform, bool) {
	for _, t := range p.transforms {
		if t.typ == typ {
			return t, true
		}
	}
	return transform{}, false
}

// suite は決まった暗号の組
type suite struct {
	encr   uint16
	keyLen int // バイト
	prf    uint16
	integ  uint16
	dh     uint16
}

func (s *suite) aead() bool { return s.encr == encrAESGCM16 || s.encr == encrChaCha }

func suiteOf(p proposal) (*suite, error) {
	s := &suite{}
	e, ok := p.get(tEncr)
	if !ok {
		return nil, errors.New("暗号が選ばれていません")
	}
	s.encr, s.keyLen = e.id, e.keyLen/8
	switch s.encr {
	case encrAESCBC, encrAESGCM16:
		if s.keyLen != 16 && s.keyLen != 24 && s.keyLen != 32 {
			return nil, fmt.Errorf("AES の鍵の長さ %d には対応していません", e.keyLen)
		}
	case encrChaCha:
		s.keyLen = 32
	default:
		return nil, fmt.Errorf("暗号 %d には対応していません", s.encr)
	}
	if t, ok := p.get(tPRF); ok {
		s.prf = t.id
		if prfHash(s.prf) == nil {
			return nil, fmt.Errorf("PRF %d には対応していません", s.prf)
		}
	}
	if t, ok := p.get(tInteg); ok && t.id != 0 {
		s.integ = t.id
		if _, n := integHash(s.integ); n == 0 {
			return nil, fmt.Errorf("完全性の方式 %d には対応していません", s.integ)
		}
	}
	if !s.aead() && s.integ == 0 {
		return nil, errors.New("完全性の方式が選ばれていません")
	}
	if t, ok := p.get(tDH); ok {
		s.dh = t.id
	}
	return s, nil
}

func prfHash(id uint16) func() hash.Hash {
	switch id {
	case prfSHA1:
		return sha1.New
	case prfSHA256:
		return sha256.New
	case prfSHA384:
		return sha512.New384
	case prfSHA512:
		return sha512.New
	}
	return nil
}

// integHash は HMAC のハッシュと、切り詰めたあとの長さ
func integHash(id uint16) (func() hash.Hash, int) {
	switch id {
	case integSHA1:
		return sha1.New, 12
	case integSHA256:
		return sha256.New, 16
	case integSHA384:
		return sha512.New384, 24
	case integSHA512:
		return sha512.New, 32
	}
	return nil, 0
}

func (s *suite) prfFunc(key, data []byte) []byte {
	m := hmac.New(prfHash(s.prf), key)
	m.Write(data)
	return m.Sum(nil)
}

func (s *suite) prfLen() int { return prfHash(s.prf)().Size() }

// prfPlus は RFC 7296 2.13 の prf+
func (s *suite) prfPlus(key, seed []byte, n int) []byte {
	var out, t []byte
	for i := byte(1); len(out) < n; i++ {
		t = s.prfFunc(key, append(append(append([]byte{}, t...), seed...), i))
		out = append(out, t...)
	}
	return out[:n]
}

// encrKeyLen は暗号の鍵の長さ。AEAD は 4 バイトの salt を含む
func (s *suite) encrKeyLen() int {
	if s.aead() {
		return s.keyLen + 4
	}
	return s.keyLen
}

func (s *suite) integKeyLen() int {
	if s.aead() {
		return 0
	}
	h, _ := integHash(s.integ)
	return h().Size()
}

// cipherKeys は片方向の暗号と完全性の鍵
type cipherKeys struct {
	s     *suite
	encr  []byte
	integ []byte
}

func (k cipherKeys) aeadCipher() (cipher.AEAD, []byte, error) {
	key, salt := k.encr[:k.s.keyLen], k.encr[k.s.keyLen:]
	if k.s.encr == encrChaCha {
		a, err := chacha20poly1305.New(key)
		return a, salt, err
	}
	blk, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	a, err := cipher.NewGCM(blk)
	return a, salt, err
}

// seal は、ad に続く平文を暗号にし、IV と暗号文と ICV を返す。CBC の平文はもう詰め物を付けたもの
func (k cipherKeys) seal(ad func(encLen int) []byte, plain []byte) ([]byte, error) {
	if k.s.aead() {
		a, salt, err := k.aeadCipher()
		if err != nil {
			return nil, err
		}
		iv := make([]byte, 8)
		rand.Read(iv)
		encLen := 8 + len(plain) + a.Overhead()
		ct := a.Seal(nil, append(append([]byte{}, salt...), iv...), plain, ad(encLen))
		return append(iv, ct...), nil
	}
	blk, err := aes.NewCipher(k.encr)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, aes.BlockSize)
	rand.Read(iv)
	ct := make([]byte, len(plain))
	cipher.NewCBCEncrypter(blk, iv).CryptBlocks(ct, plain)
	h, n := integHash(k.s.integ)
	encLen := len(iv) + len(ct) + n
	m := hmac.New(h, k.integ)
	m.Write(ad(encLen))
	m.Write(iv)
	m.Write(ct)
	return append(append(iv, ct...), m.Sum(nil)[:n]...), nil
}

var errICV = errors.New("完全性の確かめに失敗しました")

// open は、ad のあとの IV と暗号文と ICV を開き、平文を返す
func (k cipherKeys) open(ad, b []byte) ([]byte, error) {
	if k.s.aead() {
		a, salt, err := k.aeadCipher()
		if err != nil {
			return nil, err
		}
		if len(b) < 8+a.Overhead() {
			return nil, errMalformed
		}
		plain, err := a.Open(nil, append(append([]byte{}, salt...), b[:8]...), b[8:], ad)
		if err != nil {
			return nil, errICV
		}
		return plain, nil
	}
	h, n := integHash(k.s.integ)
	if len(b) < aes.BlockSize+n || (len(b)-n)%aes.BlockSize != 0 {
		return nil, errMalformed
	}
	body, icv := b[:len(b)-n], b[len(b)-n:]
	m := hmac.New(h, k.integ)
	m.Write(ad)
	m.Write(body)
	if !hmac.Equal(m.Sum(nil)[:n], icv) {
		return nil, errICV
	}
	blk, err := aes.NewCipher(k.encr)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(body)-aes.BlockSize)
	cipher.NewCBCDecrypter(blk, body[:aes.BlockSize]).CryptBlocks(plain, body[aes.BlockSize:])
	return plain, nil
}

// dhKey は鍵の交換の自分の側の鍵
type dhKey struct {
	group uint16
	pub   []byte
	priv  func(peer []byte) ([]byte, error)
}

// MODP の素数 (RFC 7296 付録 B、RFC 3526)
var modp = map[uint16]string{
	2:  "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD129024E088A67CC74020BBEA63B139B22514A08798E3404DDEF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7EDEE386BFB5A899FA5AE9F24117C4B1FE649286651ECE65381FFFFFFFFFFFFFFFF",
	14: "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD129024E088A67CC74020BBEA63B139B22514A08798E3404DDEF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7EDEE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3DC2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F83655D23DCA3AD961C62F356208552BB9ED529077096966D670C354E4ABC9804F1746C08CA18217C32905E462E36CE3BE39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9DE2BCBF6955817183995497CEA956AE515D2261898FA051015728E5A8AACAA68FFFFFFFFFFFFFFFF",
}

// NewDH は鍵の交換の自分の側を作る。L2TP/IPsec の IKEv1 でも使う。pub を相手に送り、相手の値を shared に渡す
func NewDH(group uint16) (pub []byte, shared func(peer []byte) ([]byte, error), err error) {
	k, err := newDH(group)
	if err != nil {
		return nil, nil, err
	}
	return k.pub, k.priv, nil
}

func newDH(group uint16) (*dhKey, error) {
	var curve ecdh.Curve
	switch group {
	case 19:
		curve = ecdh.P256()
	case 20:
		curve = ecdh.P384()
	case 21:
		curve = ecdh.P521()
	case 31:
		curve = ecdh.X25519()
	case 2, 14:
		p, _ := new(big.Int).SetString(modp[group], 16)
		size := (p.BitLen() + 7) / 8
		x, err := rand.Int(rand.Reader, p)
		if err != nil {
			return nil, err
		}
		pub := new(big.Int).Exp(big.NewInt(2), x, p).FillBytes(make([]byte, size))
		return &dhKey{group: group, pub: pub, priv: func(peer []byte) ([]byte, error) {
			y := new(big.Int).SetBytes(peer)
			if y.Cmp(big.NewInt(1)) <= 0 || y.Cmp(new(big.Int).Sub(p, big.NewInt(1))) >= 0 {
				return nil, errors.New("相手の DH の値が正しくありません")
			}
			return new(big.Int).Exp(y, x, p).FillBytes(make([]byte, size)), nil
		}}, nil
	default:
		return nil, fmt.Errorf("DH グループ %d には対応していません", group)
	}
	k, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	pub := k.PublicKey().Bytes()
	nist := group != 31
	if nist {
		pub = pub[1:] // RFC 5903 は x と y だけを送る。先頭の 0x04 を外す
	}
	return &dhKey{group: group, pub: pub, priv: func(peer []byte) ([]byte, error) {
		if nist {
			peer = append([]byte{4}, peer...)
		}
		pk, err := curve.NewPublicKey(peer)
		if err != nil {
			return nil, err
		}
		return k.ECDH(pk)
	}}, nil
}
