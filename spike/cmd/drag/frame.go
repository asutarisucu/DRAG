//go:build linux

package main

import (
	"encoding/binary"
	"fmt"
	"io"
)

// 保持役とエージェントの間で送るフレーム。種類 1 バイト、長さ 4 バイト、本体の順
const (
	frameInput  = 'i' // エージェント → 保持役。キー入力
	frameResize = 'r' // エージェント → 保持役。cols と rows を uint16 で
	frameOutput = 'o' // 保持役 → エージェント。シェルの出力
	frameExit   = 'x' // 保持役 → エージェント。シェルが終わった。終了コードを int32 で
)

const maxFrame = 4 << 20

func encodeFrame(typ byte, p []byte) []byte {
	b := make([]byte, 5+len(p))
	b[0] = typ
	binary.BigEndian.PutUint32(b[1:], uint32(len(p)))
	copy(b[5:], p)
	return b
}

func writeFrame(w io.Writer, typ byte, p []byte) error {
	_, err := w.Write(encodeFrame(typ, p))
	return err
}

func readFrame(r io.Reader) (byte, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > maxFrame {
		return 0, nil, fmt.Errorf("frame too large: %d", n)
	}
	p := make([]byte, n)
	_, err := io.ReadFull(r, p)
	return hdr[0], p, err
}
