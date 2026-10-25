package ovpn

import "errors"

// サーバーが圧縮して送ってきたパケットを戻す。DRAG からは圧縮して送らない。
// 圧縮の印は OpenVPN v2.6.23 の src/openvpn/comp.h、lzo.c、comp-lz4.c に合わせた
const (
	lzoCompressByte = 0x66 // comp-lzo、compress lzo
	lz4CompressByte = 0x69 // compress lz4 (先頭と末尾を入れ替える形)
	v2LZ4Byte       = 1    // compress lz4-v2。0x50 の次の1バイト
	v2LZOByte       = 2

	maxDecompressed = 65535
)

var errDecompress = errors.New("圧縮したパケットを戻せません")

// lzo1xDecompress は LZO1X の圧縮を戻す。形式は Linux の Documentation/staging/lzo.rst (v6.12) の説明による
func lzo1xDecompress(in []byte) ([]byte, error) {
	out := make([]byte, 0, 2*len(in)+64)
	ip := 0
	byteAt := func() (int, bool) {
		if ip >= len(in) {
			return 0, false
		}
		ip++
		return int(in[ip-1]), true
	}
	// 長さを、足りないぶんの 0 の並び (1 つで 255) と最後のバイトで伸ばす
	extend := func(base int) (int, bool) {
		n := 0
		for {
			b, ok := byteAt()
			if !ok {
				return 0, false
			}
			if b != 0 {
				return n + base + b, true
			}
			n += 255
		}
	}
	literals := func(n int) bool {
		if n < 0 || ip+n > len(in) || len(out)+n > maxDecompressed {
			return false
		}
		out = append(out, in[ip:ip+n]...)
		ip += n
		return true
	}
	copyMatch := func(dist, n int) bool {
		if dist <= 0 || dist > len(out) || len(out)+n > maxDecompressed {
			return false
		}
		for i := 0; i < n; i++ { // 重なることがあるので 1 バイトずつ
			out = append(out, out[len(out)-dist])
		}
		return true
	}

	state := 0 // 直前に写した文字の数。0、1〜3、4 (4 以上)
	if len(in) > 0 && in[0] > 17 {
		t := int(in[0]) - 17
		ip = 1
		if !literals(t) {
			return nil, errDecompress
		}
		state = min(t, 4)
	}
	for {
		t, ok := byteAt()
		if !ok {
			return nil, errDecompress
		}
		var dist, n int
		switch {
		case t < 16 && state == 0:
			// 4 文字以上をそのまま写す
			n = t
			if n == 0 {
				if n, ok = extend(15); !ok {
					return nil, errDecompress
				}
			}
			if !literals(n + 3) {
				return nil, errDecompress
			}
			state = 4
			continue
		case t < 16:
			h, ok := byteAt()
			if !ok {
				return nil, errDecompress
			}
			if state == 4 { // 2〜3 KB 前から 3 バイト
				dist, n = (h<<2)+(t>>2)+2049, 3
			} else { // 1 KB 以内から 2 バイト
				dist, n = (h<<2)+(t>>2)+1, 2
			}
		case t >= 64:
			h, ok := byteAt()
			if !ok {
				return nil, errDecompress
			}
			dist, n = (h<<3)+((t>>2)&7)+1, (t>>5)+1
		case t >= 32:
			n = t & 31
			if n == 0 {
				if n, ok = extend(31); !ok {
					return nil, errDecompress
				}
			}
			n += 2
			if ip+2 > len(in) {
				return nil, errDecompress
			}
			dist = (int(in[ip])>>2 | int(in[ip+1])<<6) + 1
			ip += 2
		default: // 16..31
			n = t & 7
			if n == 0 {
				if n, ok = extend(7); !ok {
					return nil, errDecompress
				}
			}
			n += 2
			if ip+2 > len(in) {
				return nil, errDecompress
			}
			dist = 16384 + (t&8)<<11 + (int(in[ip])>>2 | int(in[ip+1])<<6)
			ip += 2
			if dist == 16384 { // 終わりの印
				if ip != len(in) {
					return nil, errDecompress
				}
				return out, nil
			}
		}
		if !copyMatch(dist, n) {
			return nil, errDecompress
		}
		// 続けて写す文字の数は、2 つ前のバイトの下 2 ビット
		state = int(in[ip-2]) & 3
		if !literals(state) {
			return nil, errDecompress
		}
	}
}

// lz4Decompress は LZ4 のブロックを戻す。形式は lz4 v1.10.0 の doc/lz4_Block_format.md による
func lz4Decompress(in []byte) ([]byte, error) {
	out := make([]byte, 0, 2*len(in)+64)
	ip := 0
	length := func(n int) (int, bool) {
		if n != 15 {
			return n, true
		}
		for {
			if ip >= len(in) {
				return 0, false
			}
			b := int(in[ip])
			ip++
			n += b
			if b != 255 {
				return n, true
			}
		}
	}
	for ip < len(in) {
		token := int(in[ip])
		ip++
		lit, ok := length(token >> 4)
		if !ok || ip+lit > len(in) || len(out)+lit > maxDecompressed {
			return nil, errDecompress
		}
		out = append(out, in[ip:ip+lit]...)
		ip += lit
		if ip == len(in) { // 最後の並びは文字だけ
			return out, nil
		}
		if ip+2 > len(in) {
			return nil, errDecompress
		}
		dist := int(in[ip]) | int(in[ip+1])<<8
		ip += 2
		n, ok := length(token & 15)
		n += 4
		if !ok || dist == 0 || dist > len(out) || len(out)+n > maxDecompressed {
			return nil, errDecompress
		}
		for i := 0; i < n; i++ {
			out = append(out, out[len(out)-dist])
		}
	}
	return nil, errDecompress
}
