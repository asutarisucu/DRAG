package ovpn

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"sync"

	"golang.org/x/crypto/chacha20poly1305"
)

// supportedCiphers は IV_CIPHERS で伝えるデータチャネルの暗号。サーバーはこの中から選んで送ってくる
var supportedCiphers = []string{"AES-256-GCM", "AES-128-GCM", "AES-192-GCM", "CHACHA20-POLY1305", "AES-256-CBC", "AES-192-CBC", "AES-128-CBC"}

// dataKeys は1つの鍵の世代のデータチャネルの鍵。km は 256 バイトの鍵の素で、
// クライアントは前半 (暗号 64 + HMAC 64) で送り、後半で受ける
type dataKeys struct {
	aeadOut, aeadIn cipher.AEAD
	ivOut, ivIn     []byte // AEAD の IV のうち、パケットに載らない 8 バイト

	blkOut, blkIn cipher.Block // CBC
	hash          func() hash.Hash
	macOut, macIn []byte

	mu     sync.Mutex
	nextID uint32
	replay replayWindow
}

func newDataKeys(name, auth string, km []byte) (*dataKeys, error) {
	k := &dataKeys{}
	out, in := km[:128], km[128:]
	var mk func(key []byte) (cipher.AEAD, error)
	keyLen := 0
	switch name {
	case "AES-128-GCM", "AES-192-GCM", "AES-256-GCM":
		keyLen = map[string]int{"AES-128-GCM": 16, "AES-192-GCM": 24, "AES-256-GCM": 32}[name]
		mk = func(key []byte) (cipher.AEAD, error) {
			b, err := aes.NewCipher(key)
			if err != nil {
				return nil, err
			}
			return cipher.NewGCM(b)
		}
	case "CHACHA20-POLY1305":
		keyLen, mk = 32, chacha20poly1305.New
	case "AES-128-CBC", "AES-192-CBC", "AES-256-CBC":
		keyLen = map[string]int{"AES-128-CBC": 16, "AES-192-CBC": 24, "AES-256-CBC": 32}[name]
		h, ok := digests[auth]
		if !ok {
			return nil, fmt.Errorf("auth %s には対応していません", auth)
		}
		var err error
		if k.blkOut, err = aes.NewCipher(out[:keyLen]); err != nil {
			return nil, err
		}
		k.blkIn, _ = aes.NewCipher(in[:keyLen])
		k.hash = h
		n := h().Size()
		k.macOut, k.macIn = out[64:64+n], in[64:64+n]
		return k, nil
	default:
		return nil, fmt.Errorf("データチャネルの暗号 %s には対応していません", name)
	}
	var err error
	if k.aeadOut, err = mk(out[:keyLen]); err != nil {
		return nil, err
	}
	k.aeadIn, _ = mk(in[:keyLen])
	k.ivOut, k.ivIn = out[64:72], in[64:72]
	return k, nil
}

// seal はデータのパケットを作る。head は opcode と key_id (と DATA_V2 なら peer-id) の部分
func (k *dataKeys) seal(head, payload []byte) []byte {
	k.mu.Lock()
	k.nextID++
	id := k.nextID
	k.mu.Unlock()
	pid := binary.BigEndian.AppendUint32(nil, id)
	if k.aeadOut != nil {
		ad := pid
		if len(head) == 4 {
			ad = append(append([]byte{}, head...), pid...)
		}
		ct := k.aeadOut.Seal(nil, append(append([]byte{}, pid...), k.ivOut...), payload, ad)
		n := len(ct) - k.aeadOut.Overhead()
		out := append(append([]byte{}, head...), pid...)
		out = append(out, ct[n:]...) // タグを先に置く
		return append(out, ct[:n]...)
	}
	// CBC。packet_id を暗号の中に入れ、IV と暗号文に HMAC を付ける
	plain := append(pid, payload...)
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	for i := 0; i < pad; i++ {
		plain = append(plain, byte(pad))
	}
	iv := make([]byte, aes.BlockSize)
	rand.Read(iv)
	ct := make([]byte, len(plain))
	cipher.NewCBCEncrypter(k.blkOut, iv).CryptBlocks(ct, plain)
	m := hmac.New(k.hash, k.macOut)
	m.Write(iv)
	m.Write(ct)
	out := append(append([]byte{}, head...), m.Sum(nil)...)
	out = append(out, iv...)
	return append(out, ct...)
}

var errReplay = errors.New("同じデータのパケットをもう一度受けました")

// open はデータのパケットを開く。b は head のあとの部分
func (k *dataKeys) open(head, b []byte) ([]byte, error) {
	if k.aeadIn != nil {
		if len(b) < 4+16 {
			return nil, errShort
		}
		pid, tag, ct := b[:4], b[4:20], b[20:]
		ad := pid
		if len(head) == 4 {
			ad = append(append([]byte{}, head...), pid...)
		}
		plain, err := k.aeadIn.Open(nil, append(append([]byte{}, pid...), k.ivIn...), append(append([]byte{}, ct...), tag...), ad)
		if err != nil {
			return nil, err
		}
		if !k.accept(binary.BigEndian.Uint32(pid)) {
			return nil, errReplay
		}
		return plain, nil
	}
	n := k.hash().Size()
	if len(b) < n+aes.BlockSize*2 || (len(b)-n)%aes.BlockSize != 0 {
		return nil, errShort
	}
	mac, iv, ct := b[:n], b[n:n+aes.BlockSize], b[n+aes.BlockSize:]
	m := hmac.New(k.hash, k.macIn)
	m.Write(iv)
	m.Write(ct)
	if !hmac.Equal(m.Sum(nil), mac) {
		return nil, errors.New("データのパケットの HMAC が合いません")
	}
	plain := make([]byte, len(ct))
	cipher.NewCBCDecrypter(k.blkIn, iv).CryptBlocks(plain, ct)
	pad := int(plain[len(plain)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(plain)-4 {
		return nil, errors.New("データのパケットの詰め物が違います")
	}
	plain = plain[:len(plain)-pad]
	if !k.accept(binary.BigEndian.Uint32(plain)) {
		return nil, errReplay
	}
	return plain[4:], nil
}

func (k *dataKeys) accept(id uint32) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.replay.accept(id)
}

// replayWindow は、受けた packet_id のうち、いちばん大きいものとその手前 64 個を覚える
type replayWindow struct {
	top  uint32
	seen uint64 // ビット i が top-i を受けたか
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
