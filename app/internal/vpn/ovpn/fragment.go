package ovpn

import (
	"encoding/binary"
	"sync"
	"time"
)

// .ovpn の fragment で、データのパケットを分けて送り、分かれて届いたものをつなぐ。
// 頭の 4 バイトの形は OpenVPN v2.6.23 の src/openvpn/fragment.h と fragment.c に合わせた。
// 送るときは圧縮の印を付けたあとに分け、受けるときはつないでから圧縮の印を外す (forward.c)
const (
	fragWhole   = 0
	fragNotLast = 1
	fragLast    = 2

	fragMax = 32 // 分けられる数 (番号は 5 ビット)
	// fragOverhead は、暗号と OpenVPN の頭のぶん。CBC と HMAC の組でも収まるように多めに取る
	fragOverhead = 120
)

type fragHeader uint32

func (h fragHeader) typ() int  { return int(h & 3) }
func (h fragHeader) seq() int  { return int(h>>2) & 0xff }
func (h fragHeader) id() int   { return int(h>>10) & 0x1f }
func (h fragHeader) size() int { return int(h>>15) & 0x3fff << 2 }
func putFrag(typ, seq, id, size int) []byte {
	h := uint32(typ&3) | uint32(seq&0xff)<<2 | uint32(id&0x1f)<<10
	if typ == fragLast {
		h |= uint32(size>>2&0x3fff) << 15
	}
	return binary.BigEndian.AppendUint32(nil, h)
}

// fragmenter は、分けて送る番号と、受けてつなぎかけのもの
type fragmenter struct {
	max  int // 1 つに入れる大きさ。4 の倍数
	mu   sync.Mutex
	seq  int
	part map[int]*fragPart // 受けるほうは1つのゴルーチンだけが触る
}

type fragPart struct {
	size  int
	got   uint32
	buf   []byte
	start time.Time
}

func newFragmenter(fragment int) *fragmenter {
	return &fragmenter{max: max(fragment-fragOverhead, 64) &^ 3, part: map[int]*fragPart{}}
}

// split は送るものに頭を付け、大きければ分ける
func (f *fragmenter) split(p []byte) [][]byte {
	if len(p) <= f.max {
		return [][]byte{append(putFrag(fragWhole, 0, 0, 0), p...)}
	}
	f.mu.Lock()
	f.seq = (f.seq + 1) & 0xff
	seq := f.seq
	f.mu.Unlock()
	var out [][]byte
	for id := 0; len(p) > 0 && id < fragMax; id++ {
		n, typ := f.max, fragNotLast
		if len(p) <= n {
			n, typ = len(p), fragLast
		}
		out = append(out, append(putFrag(typ, seq, id, f.max), p[:n]...))
		p = p[n:]
	}
	if len(p) > 0 {
		return nil // 32 に分けても入らない
	}
	return out
}

// join は届いたものの頭を外す。分かれたものは揃うまで nil を返す
func (f *fragmenter) join(p []byte) []byte {
	if len(p) < 4 {
		return nil
	}
	h := fragHeader(binary.BigEndian.Uint32(p))
	p = p[4:]
	switch h.typ() {
	case fragWhole:
		return p
	case fragNotLast, fragLast:
	default:
		return nil
	}
	size := len(p)
	if h.typ() == fragLast {
		size = h.size()
	}
	if size == 0 || size&3 != 0 {
		return nil
	}
	now := time.Now()
	for k, o := range f.part {
		if now.Sub(o.start) > 10*time.Second {
			delete(f.part, k)
		}
	}
	fp := f.part[h.seq()]
	if fp == nil || fp.size != size {
		fp = &fragPart{size: size, start: now}
		f.part[h.seq()] = fp
	}
	off := h.id() * size
	if end := off + len(p); end > len(fp.buf) {
		fp.buf = append(fp.buf, make([]byte, end-len(fp.buf))...)
	}
	copy(fp.buf[off:], p)
	if h.typ() == fragLast {
		fp.got |= 0xffffffff << h.id()
	} else {
		fp.got |= 1 << h.id()
	}
	if fp.got != 0xffffffff {
		return nil
	}
	delete(f.part, h.seq())
	return fp.buf
}
