package l2tp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// L2TP の制御メッセージの種類と AVP (RFC 2661 3.2、4.4)
const (
	msgSCCRQ   = 1
	msgSCCRP   = 2
	msgSCCCN   = 3
	msgStopCCN = 4
	msgHello   = 6
	msgICRQ    = 10
	msgICRP    = 11
	msgICCN    = 12
	msgCDN     = 14

	avpMessageType   = 0
	avpResultCode    = 1
	avpProtocolVer   = 2
	avpFramingCap    = 3
	avpHostName      = 7
	avpTunnelID      = 9
	avpSessionID     = 14
	avpCallSerial    = 15
	avpFramingType   = 19
	avpConnectSpeed  = 24
	flagControl      = 0xc802 // T、L、S のビットと版 2
	flagData         = 0x0002
	flagDataSequence = 0x0802
)

// ctlSent は、相手が受け取ったと返すまで送り直す制御メッセージ
type ctlSent struct {
	ns    uint16
	raw   []byte
	next  time.Time
	tries int
}

// ctlMsg は受けた制御メッセージ。AVP は種類ごとの値
type ctlMsg struct {
	typ  uint16
	avps map[uint16][]byte
}

func avp(typ uint16, v []byte) []byte {
	b := binary.BigEndian.AppendUint16(nil, 0x8000|uint16(6+len(v))) // M のビットを立てる
	b = binary.BigEndian.AppendUint16(b, 0)
	b = binary.BigEndian.AppendUint16(b, typ)
	return append(b, v...)
}

func u16(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }

func parseAVPs(b []byte) (ctlMsg, bool) {
	m := ctlMsg{avps: map[uint16][]byte{}}
	for len(b) >= 6 {
		n := int(binary.BigEndian.Uint16(b) & 0x03ff)
		if n < 6 || n > len(b) {
			return m, false
		}
		// 隠した AVP (H のビット) は使わない
		if binary.BigEndian.Uint16(b[2:4]) == 0 && b[0]&0x40 == 0 {
			m.avps[binary.BigEndian.Uint16(b[4:6])] = b[6:n]
		}
		b = b[n:]
	}
	v, ok := m.avps[avpMessageType]
	if !ok || len(v) != 2 {
		return m, false
	}
	m.typ = binary.BigEndian.Uint16(v)
	return m, true
}

// before は、16 ビットで回る番号 a が b より前か
func before(a, b uint16) bool { return int16(a-b) < 0 }

// sendL2TP は L2TP のパケットを UDP (1701 番から 1701 番) に入れ、ESP で包んで送る
func (c *Client) sendL2TP(p []byte) error {
	c.mu.Lock()
	var out *espSA
	if n := len(c.outbound); n > 0 {
		out = c.outbound[n-1]
	}
	c.mu.Unlock()
	if out == nil {
		return errors.New("ESP の SA がありません")
	}
	udp := binary.BigEndian.AppendUint16(nil, l2tpPort)
	udp = binary.BigEndian.AppendUint16(udp, l2tpPort)
	udp = binary.BigEndian.AppendUint16(udp, uint16(8+len(p)))
	udp = append(udp, 0, 0) // IPv4 の UDP は checksum を 0 にできる (RFC 3948 3.1.2)
	return c.write(out.seal(protoUDP, append(udp, p...)))
}

// sendControl は制御メッセージを送り、受け取ったと返るまで送り直すように残す
func (c *Client) sendControl(session uint16, avps ...[]byte) error {
	var body []byte
	for _, a := range avps {
		body = append(body, a...)
	}
	c.mu.Lock()
	ns, nr := c.ns, c.nr
	c.ns++
	b := binary.BigEndian.AppendUint16(nil, flagControl)
	b = binary.BigEndian.AppendUint16(b, uint16(12+len(body)))
	b = binary.BigEndian.AppendUint16(b, c.peerTunnel)
	b = binary.BigEndian.AppendUint16(b, session)
	b = binary.BigEndian.AppendUint16(b, ns)
	b = binary.BigEndian.AppendUint16(b, nr)
	b = append(b, body...)
	c.unacked = append(c.unacked, ctlSent{ns: ns, raw: b, next: time.Now().Add(time.Second)})
	c.mu.Unlock()
	return c.sendL2TP(b)
}

// sendZLB は中身の無い制御メッセージで、受け取ったことだけを返す
func (c *Client) sendZLB() {
	c.mu.Lock()
	b := binary.BigEndian.AppendUint16(nil, flagControl)
	b = binary.BigEndian.AppendUint16(b, 12)
	b = binary.BigEndian.AppendUint16(b, c.peerTunnel)
	b = binary.BigEndian.AppendUint16(b, 0)
	b = binary.BigEndian.AppendUint16(b, c.ns)
	b = binary.BigEndian.AppendUint16(b, c.nr)
	c.mu.Unlock()
	c.sendL2TP(b)
}

// retransmit は、返事の無い制御メッセージを 1、2、4、8 秒の間を空けて送り直す
func (c *Client) retransmit(now time.Time) {
	c.mu.Lock()
	var again [][]byte
	failed := false
	for i := range c.unacked {
		u := &c.unacked[i]
		if now.Before(u.next) {
			continue
		}
		if u.tries++; u.tries > 5 {
			failed = true
			break
		}
		u.next = now.Add(time.Second << u.tries)
		again = append(again, u.raw)
	}
	c.mu.Unlock()
	if failed {
		c.fail(errors.New("VPN のサーバーから L2TP の返事がありません"))
		return
	}
	for _, b := range again {
		c.sendL2TP(b)
	}
}

// handleL2TP は ESP から出てきた L2TP のパケットを受ける
func (c *Client) handleL2TP(p []byte) {
	if len(p) < 6 || binary.BigEndian.Uint16(p)&0x000f != 2 {
		return
	}
	flags, off := binary.BigEndian.Uint16(p), 2
	if flags&0x4000 != 0 {
		off += 2
	}
	if len(p) < off+4 {
		return
	}
	session := binary.BigEndian.Uint16(p[off+2:])
	off += 4
	var ns, nr uint16
	if flags&0x0800 != 0 {
		if len(p) < off+4 {
			return
		}
		ns, nr = binary.BigEndian.Uint16(p[off:]), binary.BigEndian.Uint16(p[off+2:])
		off += 4
	}
	if flags&0x0200 != 0 {
		if len(p) < off+2 {
			return
		}
		off += 2 + int(binary.BigEndian.Uint16(p[off:]))
	}
	if off > len(p) {
		return
	}
	if flags&0x8000 == 0 {
		c.mu.Lock()
		ok := session == c.session
		// サーバーが番号を付けてきたら、こちらも付ける (RFC 2661 5.4)
		c.seqData = c.seqData || flags&0x0800 != 0
		c.mu.Unlock()
		if ok && c.ppp != nil {
			c.ppp.Input(p[off:])
		}
		return
	}
	c.mu.Lock()
	kept := c.unacked[:0]
	for _, u := range c.unacked {
		if !before(u.ns, nr) {
			kept = append(kept, u)
		}
	}
	c.unacked = kept
	if off == len(p) { // ZLB
		c.mu.Unlock()
		return
	}
	if ns != c.nr {
		// 送り直しか順番違い。受けた番号を返し直す
		c.mu.Unlock()
		c.sendZLB()
		return
	}
	c.nr++
	c.mu.Unlock()
	c.sendZLB()
	m, ok := parseAVPs(p[off:])
	if !ok {
		return
	}
	switch m.typ {
	case msgStopCCN:
		c.fail(fmt.Errorf("VPN のサーバーが L2TP を終えました%s", resultText(m)))
	case msgCDN:
		c.fail(fmt.Errorf("VPN のサーバーが L2TP のセッションを終えました%s", resultText(m)))
	case msgHello:
	default:
		select {
		case c.ctlIn <- m:
		default:
		}
	}
}

func resultText(m ctlMsg) string {
	r := m.avps[avpResultCode]
	if len(r) < 2 {
		return ""
	}
	s := fmt.Sprintf(" (結果 %d", binary.BigEndian.Uint16(r))
	if len(r) > 4 {
		s += " " + string(r[4:])
	}
	return s + ")"
}

// waitControl は typ の制御メッセージが届くまで待つ
func (c *Client) waitControl(ctx context.Context, typ uint16) (ctlMsg, error) {
	for {
		select {
		case m := <-c.ctlIn:
			if m.typ == typ {
				return m, nil
			}
		case <-ctx.Done():
			return ctlMsg{}, errors.New("VPN のサーバーから L2TP の返事がありません")
		case <-c.done:
			return ctlMsg{}, c.err
		}
	}
}

// startL2TP は L2TP のトンネルとセッションを開く (RFC 2661 5.1、6)
func (c *Client) startL2TP(ctx context.Context) error {
	c.mu.Lock()
	c.tunnel, c.session = uint16(randSPI()), uint16(randSPI())
	if c.tunnel == 0 {
		c.tunnel = 1
	}
	if c.session == 0 {
		c.session = 1
	}
	tunnel, session := c.tunnel, c.session
	c.mu.Unlock()
	if err := c.sendControl(0,
		avp(avpMessageType, u16(msgSCCRQ)),
		avp(avpProtocolVer, []byte{1, 0}),
		avp(avpHostName, []byte("DRAG")),
		avp(avpFramingCap, be32(3)),
		avp(avpTunnelID, u16(tunnel)),
	); err != nil {
		return err
	}
	m, err := c.waitControl(ctx, msgSCCRP)
	if err != nil {
		return err
	}
	peer := m.avps[avpTunnelID]
	if len(peer) != 2 {
		return errors.New("VPN のサーバーが L2TP のトンネルの番号を返しません")
	}
	c.mu.Lock()
	c.peerTunnel = binary.BigEndian.Uint16(peer)
	c.mu.Unlock()
	if err := c.sendControl(0, avp(avpMessageType, u16(msgSCCCN))); err != nil {
		return err
	}
	if err := c.sendControl(0,
		avp(avpMessageType, u16(msgICRQ)),
		avp(avpSessionID, u16(session)),
		avp(avpCallSerial, be32(1)),
	); err != nil {
		return err
	}
	m, err = c.waitControl(ctx, msgICRP)
	if err != nil {
		return err
	}
	peerSession := m.avps[avpSessionID]
	if len(peerSession) != 2 {
		return errors.New("VPN のサーバーが L2TP のセッションの番号を返しません")
	}
	c.mu.Lock()
	c.peerSession = binary.BigEndian.Uint16(peerSession)
	c.mu.Unlock()
	return c.sendControl(c.peerSession,
		avp(avpMessageType, u16(msgICCN)),
		avp(avpConnectSpeed, be32(100000000)),
		avp(avpFramingType, be32(1)),
	)
}

// sendFrame は PPP のフレームを L2TP のデータメッセージで送る
func (c *Client) sendFrame(frame []byte) error {
	c.mu.Lock()
	flags := uint16(flagData)
	if c.seqData {
		flags = flagDataSequence
	}
	b := binary.BigEndian.AppendUint16(nil, flags)
	b = binary.BigEndian.AppendUint16(b, c.peerTunnel)
	b = binary.BigEndian.AppendUint16(b, c.peerSession)
	if c.seqData {
		b = binary.BigEndian.AppendUint16(b, c.dataNs)
		b = binary.BigEndian.AppendUint16(b, 0)
		c.dataNs++
	}
	c.mu.Unlock()
	return c.sendL2TP(append(b, frame...))
}
