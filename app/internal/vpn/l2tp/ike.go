package l2tp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/asutarisucu/DRAG/app/internal/vpn/ikev2"
)

// vidNATT は NAT-T (RFC 3947) に対応していると伝える Vendor ID。MD5("RFC 3947")
var vidNATT = func() []byte { s := md5.Sum([]byte("RFC 3947")); return s[:] }()

const (
	p1Lifetime = 28800 // Phase 1 の SA の寿命 (秒)
	l2tpPort   = 1701
	protoUDP   = 17
)

// ikeMsg はサーバーから届いた IKE のメッセージ。err は、Phase 1 の途中でサーバーが断ってきたとき
type ikeMsg struct {
	h    header
	body []byte
	err  error
}

type waiter struct {
	exch  byte
	msgID uint32
	ch    chan ikeMsg
}

func be32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }

// withHash は HASH の payload を先頭に置き、後ろに rest をつなぐ
func withHash(next byte, hash, rest []byte) []byte {
	b := []byte{next, 0}
	b = binary.BigEndian.AppendUint16(b, uint16(4+len(hash)))
	return append(append(b, hash...), rest...)
}

func (c *Client) writeIKE(h header, body []byte) error {
	return c.write(append(append([]byte{0, 0, 0, 0}, h.encode(len(body))...), body...))
}

// exchange は IKE のメッセージを送り、accept が受け取る返事が来るまで送り直す
func (c *Client) exchange(ctx context.Context, h header, body []byte, accept func(ikeMsg) bool, limit time.Duration) (ikeMsg, error) {
	w := &waiter{exch: h.exch, msgID: h.msgID, ch: make(chan ikeMsg, 8)}
	c.mu.Lock()
	c.waiting = w
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.waiting == w {
			c.waiting = nil
		}
		c.mu.Unlock()
	}()
	deadline := time.Now().Add(limit)
	wait := 500 * time.Millisecond
	for {
		if err := c.writeIKE(h, body); err != nil {
			return ikeMsg{}, err
		}
		t := time.NewTimer(wait)
	recv:
		for {
			select {
			case m := <-w.ch:
				if m.err != nil {
					t.Stop()
					return ikeMsg{}, m.err
				}
				if accept(m) {
					t.Stop()
					return m, nil
				}
			case <-t.C:
				break recv
			case <-ctx.Done():
				t.Stop()
				return ikeMsg{}, errNoReply
			case <-c.done:
				t.Stop()
				return ikeMsg{}, c.err
			}
		}
		if time.Now().After(deadline) {
			return ikeMsg{}, errNoReply
		}
		wait = min(wait*2, 4*time.Second)
	}
}

var errNoReply = errors.New("VPN のサーバーから返事がありません")

// mainMode は Main Mode (RFC 2409 5.4) で事前共有キーを使い、ISAKMP の SA を作る。
// NAT-T のポートで始め、自分の側の NAT-D をわざと合わせず、ESP を UDP で包ませる
func (c *Client) mainMode(ctx context.Context) error {
	copy(c.ispi[:], randBytes(8))
	var ts []transform
	for _, s := range p1Suites {
		ts = append(ts, transform{id: keyIKE, attrs: s.attrs(p1Lifetime)})
	}
	sai := encodeSA(proposal{proto: protoISAKMP, transforms: ts})
	h := header{ispi: c.ispi, exch: exMain, next: plSA}
	m, err := c.exchange(ctx, h, chain(payload{plSA, sai}, payload{plVendor, vidNATT}), func(m ikeMsg) bool { return m.h.next == plSA }, 20*time.Second)
	if err != nil {
		return err
	}
	c.rspi = m.h.rspi
	ps, err := parse(m.h.next, m.body)
	if err != nil {
		return err
	}
	props, err := parseSA(find(ps, plSA))
	if err != nil || len(props) == 0 || len(props[0].transforms) == 0 {
		return errors.New("VPN のサーバーの Phase 1 の返事を読めません")
	}
	t := props[0].transforms[0]
	s := p1Suite{t.vals[p1Encr], t.vals[p1KeyLen], t.vals[p1Hash], t.vals[p1Group]}
	ok := false
	for _, x := range p1Suites {
		ok = ok || x == s
	}
	if !ok || t.vals[p1Auth] != authPSK {
		return fmt.Errorf("VPN のサーバーが出していない Phase 1 の組を選びました (%v)", t.vals)
	}
	natt := false
	for _, v := range findAll(ps, plVendor) {
		natt = natt || bytes.Equal(v, vidNATT)
	}
	if !natt {
		return errors.New("VPN のサーバーが NAT-T (RFC 3947) に対応していません")
	}

	gxi, shared, err := ikev2.NewDH(uint16(s.group))
	if err != nil {
		return err
	}
	ni := randBytes(nonceSize)
	hf := hashOf(s.hash)
	natd := hf()
	natd.Write(c.ispi[:])
	natd.Write(c.rspi[:])
	natd.Write(c.remote[:])
	natd.Write(binary.BigEndian.AppendUint16(nil, c.remotePort))
	h = header{ispi: c.ispi, rspi: c.rspi, exch: exMain, next: plKE}
	body := chain(payload{plKE, gxi}, payload{plNonce, ni}, payload{plNATD, natd.Sum(nil)}, payload{plNATD, randBytes(hf().Size())})
	m, err = c.exchange(ctx, h, body, func(m ikeMsg) bool { return m.h.next == plKE }, 20*time.Second)
	if err != nil {
		return err
	}
	ps, err = parse(m.h.next, m.body)
	if err != nil {
		return err
	}
	gxr, nr := find(ps, plKE), find(ps, plNonce)
	if gxr == nil || nr == nil {
		return errMalformed
	}
	gxy, err := shared(gxr)
	if err != nil {
		return err
	}
	k, err := derive(s, []byte(c.cfg.PSK), ni, nr, gxy, c.ispi, c.rspi)
	if err != nil {
		return err
	}
	ivh := hf()
	ivh.Write(gxi)
	ivh.Write(gxr)
	iv := ivh.Sum(nil)[:k.block.BlockSize()]

	idii := idPayload(c.local, 0, 0)
	hashI := prf(hf, k.skeyid, gxi, gxr, c.ispi[:], c.rspi[:], sai, idii)
	ct, iv := k.encrypt(iv, chain(payload{plID, idii}, payload{plHash, hashI}))
	h = header{ispi: c.ispi, rspi: c.rspi, exch: exMain, next: plID, flags: flagEnc}
	// 事前共有キーが違うと、サーバーはこの暗号を解けずに黙る
	m, err = c.exchange(ctx, h, ct, func(m ikeMsg) bool { return m.h.flags&flagEnc != 0 }, 8*time.Second)
	if errors.Is(err, errNoReply) {
		return errors.New("VPN のサーバーから Phase 1 の認証の返事がありません。事前共有キーを確かめてください")
	}
	if err != nil {
		return err
	}
	plain, last, err := k.decrypt(iv, m.body)
	if err != nil {
		return err
	}
	ps, err = parse(m.h.next, plain)
	idir, hashR := find(ps, plID), find(ps, plHash)
	if err != nil || idir == nil || hashR == nil {
		return errors.New("VPN のサーバーの Phase 1 の認証を読めません。事前共有キーを確かめてください")
	}
	if !hmac.Equal(hashR, prf(hf, k.skeyid, gxr, gxi, c.rspi[:], c.ispi[:], sai, idir)) {
		return errors.New("VPN のサーバーの Phase 1 の認証が合いません。事前共有キーを確かめてください")
	}
	k.lastIV = last
	c.mu.Lock()
	c.ike = k
	c.mu.Unlock()
	c.logf("L2TP/IPsec の Phase 1 ができました")
	return nil
}

// p2SuiteOf は、ESP の変換が出せる組のどれかを返す
func p2SuiteOf(t transform) (p2Suite, bool) {
	s := p2Suite{uint32(t.id), t.vals[p2KeyLen], t.vals[p2Auth]}
	if t.vals[p2Mode] != modeUDPTransport {
		return s, false
	}
	for _, x := range p2Suites {
		if x == s {
			return s, true
		}
	}
	return s, false
}

// lifeOf は ESP の SA の寿命。秒の指定が無ければ RFC 2407 4.5 の既定の 8 時間
func lifeOf(t transform) time.Duration {
	if t.life == 0 {
		return 8 * time.Hour
	}
	return time.Duration(t.life) * time.Second
}

func randSPI() uint32 {
	for {
		if v := binary.BigEndian.Uint32(randBytes(4)); v > 255 {
			return v
		}
	}
}

// quickMode は Quick Mode (RFC 2409 5.5) で、L2TP の UDP だけを包む ESP の SA の組を作る
func (c *Client) quickMode(ctx context.Context) error {
	c.mu.Lock()
	k := c.ike
	c.mu.Unlock()
	msgID, spi, ni := randSPI(), randSPI(), randBytes(nonceSize)
	var ts []transform
	for _, s := range p2Suites {
		ts = append(ts, transform{id: byte(s.encr), attrs: s.attrs(uint32(c.lifetime / time.Second))})
	}
	rest := chain(
		payload{plSA, encodeSA(proposal{proto: protoESP, spi: be32(spi), transforms: ts})},
		payload{plNonce, ni},
		payload{plID, idPayload(c.local, protoUDP, l2tpPort)},
		payload{plID, idPayload(c.remote, protoUDP, l2tpPort)},
		payload{plNATOA, natOA(c.local)},
		payload{plNATOA, natOA(c.remote)},
	)
	ct, iv := k.encrypt(k.phase2IV(msgID), withHash(plSA, prf(k.h, k.a, be32(msgID), rest), rest))
	h := header{ispi: c.ispi, rspi: c.rspi, exch: exQuick, next: plHash, flags: flagEnc, msgID: msgID}
	m, err := c.exchange(ctx, h, ct, func(ikeMsg) bool { return true }, 20*time.Second)
	if err != nil {
		return err
	}
	plain, iv, err := k.decrypt(iv, m.body)
	if err != nil {
		return err
	}
	ps, err := parse(m.h.next, plain)
	if err != nil || len(ps) == 0 || ps[0].typ != plHash {
		return errors.New("VPN のサーバーの Quick Mode の返事を読めません")
	}
	if !hmac.Equal(ps[0].body, prf(k.h, k.a, be32(msgID), ni, plain[ps[0].end:ps[len(ps)-1].end])) {
		return errHash
	}
	props, err := parseSA(find(ps, plSA))
	nr := find(ps, plNonce)
	if err != nil || nr == nil || len(props) == 0 || len(props[0].transforms) == 0 || len(props[0].spi) != 4 {
		return errors.New("VPN のサーバーの Quick Mode の返事を読めません")
	}
	t := props[0].transforms[0]
	s, ok := p2SuiteOf(t)
	if !ok {
		return fmt.Errorf("VPN のサーバーが出していない ESP の組を選びました (%d %v)", t.id, t.vals)
	}
	ct3, _ := k.encrypt(iv, withHash(0, prf(k.h, k.a, []byte{0}, be32(msgID), ni, nr), nil))
	h.next = plHash
	raw3 := append(append([]byte{0, 0, 0, 0}, h.encode(len(ct3))...), ct3...)
	c.mu.Lock()
	c.resend[msgID] = raw3
	c.mu.Unlock()
	if err := c.write(raw3); err != nil {
		return err
	}
	outSPI := binary.BigEndian.Uint32(props[0].spi)
	return c.install(k, s, lifeOf(t), spi, outSPI, ni, nr, true)
}

// install は KEYMAT から ESP の SA の組を作る。in は自分が選んだ SPI、out はサーバーが選んだ SPI
func (c *Client) install(k *isakmpCrypto, s p2Suite, life time.Duration, in, out uint32, ni, nr []byte, outNow bool) error {
	n := espKeyLen(s)
	inSA, err := newESP(s, in, k.keymat(in, ni, nr, n), life)
	if err != nil {
		return err
	}
	outSA, err := newESP(s, out, k.keymat(out, ni, nr, n), life)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.inbound[in] = inSA
	c.pair[out] = in
	if outNow {
		c.outbound = append(c.outbound, outSA)
	} else {
		c.later[in] = outSA
	}
	c.mu.Unlock()
	c.logf("ESP の SA を作りました (受ける SPI %08x、送る SPI %08x、%s)", in, out, life)
	return nil
}

// handleIKE はサーバーから届いた IKE のメッセージを振り分ける
func (c *Client) handleIKE(raw []byte) {
	h, body, err := parseHeader(raw)
	if err != nil || h.ispi != c.ispi {
		return
	}
	c.mu.Lock()
	w, k, again := c.waiting, c.ike, c.resend[h.msgID]
	c.mu.Unlock()
	switch {
	case h.exch == exInfo:
		c.onInfo(w, k, h, body)
	case w != nil && w.exch == h.exch && w.msgID == h.msgID:
		select {
		case w.ch <- ikeMsg{h: h, body: body}:
		default:
		}
	case h.exch == exQuick && k != nil:
		if again != nil && !c.responding(h.msgID) {
			// 自分の最後のメッセージが届かず、サーバーが送り直してきた
			c.write(again)
			return
		}
		c.onQuick(k, h, body)
	}
}

// qmState は、サーバーが始めた Quick Mode の途中
type qmState struct {
	ni, nr  []byte
	in, out uint32
	iv      []byte
	reply   []byte
}

func (c *Client) responding(msgID uint32) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.qm[msgID] != nil
}

// onQuick は、サーバーが SA を作り直す Quick Mode に答える
func (c *Client) onQuick(k *isakmpCrypto, h header, body []byte) {
	c.mu.Lock()
	st := c.qm[h.msgID]
	c.mu.Unlock()
	if st != nil {
		plain, _, err := k.decrypt(st.iv, body)
		if err == nil {
			ps, err := parse(h.next, plain)
			if err == nil && len(ps) > 0 && ps[0].typ == plHash && hmac.Equal(ps[0].body, prf(k.h, k.a, []byte{0}, be32(h.msgID), st.ni, st.nr)) {
				c.mu.Lock()
				if o := c.later[st.in]; o != nil {
					c.outbound = append(c.outbound, o)
					delete(c.later, st.in)
				}
				delete(c.qm, h.msgID)
				c.mu.Unlock()
				c.logf("サーバーが始めた Quick Mode が済みました")
				return
			}
		}
		// 最初のメッセージの送り直し。返事をもう一度送る
		c.write(st.reply)
		return
	}
	plain, iv, err := k.decrypt(k.phase2IV(h.msgID), body)
	if err != nil {
		return
	}
	ps, err := parse(h.next, plain)
	if err != nil || len(ps) == 0 || ps[0].typ != plHash || !hmac.Equal(ps[0].body, prf(k.h, k.a, be32(h.msgID), plain[ps[0].end:ps[len(ps)-1].end])) {
		return
	}
	ni := find(ps, plNonce)
	props, err := parseSA(find(ps, plSA))
	if err != nil || ni == nil || find(ps, plKE) != nil {
		return
	}
	var chosen *transform
	var s p2Suite
	var outSPI uint32
	for _, p := range props {
		for _, t := range p.transforms {
			if x, ok := p2SuiteOf(t); ok && p.proto == protoESP && len(p.spi) == 4 && chosen == nil {
				chosen, s, outSPI = &t, x, binary.BigEndian.Uint32(p.spi)
			}
		}
	}
	if chosen == nil {
		c.logf("サーバーが出した ESP の組を使えません")
		return
	}
	in, nr := randSPI(), randBytes(nonceSize)
	pls := []payload{
		{plSA, encodeSA(proposal{proto: protoESP, spi: be32(in), transforms: []transform{{id: chosen.id, attrs: chosen.attrs}}})},
		{plNonce, nr},
	}
	if ids := findAll(ps, plID); len(ids) == 2 {
		pls = append(pls, payload{plID, ids[0]}, payload{plID, ids[1]})
	}
	if find(ps, plNATOA) != nil {
		pls = append(pls, payload{plNATOA, natOA(c.remote)}, payload{plNATOA, natOA(c.local)})
	}
	rest := chain(pls...)
	ct, iv := k.encrypt(iv, withHash(plSA, prf(k.h, k.a, be32(h.msgID), ni, rest), rest))
	h.next = plHash
	reply := append(append([]byte{0, 0, 0, 0}, h.encode(len(ct))...), ct...)
	if err := c.install(k, s, lifeOf(*chosen), in, outSPI, ni, nr, false); err != nil {
		return
	}
	c.mu.Lock()
	c.qm[h.msgID] = &qmState{ni: ni, nr: nr, in: in, out: outSPI, iv: iv, reply: reply}
	c.mu.Unlock()
	c.write(reply)
}

// onInfo は Informational を受ける。Phase 1 の途中で断られたらそれを返し、
// 暗号にしたものは DPD に答え、消された SA を外す
func (c *Client) onInfo(w *waiter, k *isakmpCrypto, h header, body []byte) {
	if h.flags&flagEnc == 0 {
		ps, err := parse(h.next, body)
		n := find(ps, plNotify)
		if err == nil && n != nil && len(n) >= 8 && w != nil && w.exch == exMain {
			w.ch <- ikeMsg{err: fmt.Errorf("VPN のサーバーが IKE を断りました (Notify %d)", binary.BigEndian.Uint16(n[6:8]))}
		}
		return
	}
	if k == nil {
		return
	}
	plain, _, err := k.decrypt(k.phase2IV(h.msgID), body)
	if err != nil {
		return
	}
	ps, err := parse(h.next, plain)
	if err != nil || len(ps) < 2 || ps[0].typ != plHash || !hmac.Equal(ps[0].body, prf(k.h, k.a, be32(h.msgID), plain[ps[0].end:ps[len(ps)-1].end])) {
		return
	}
	for _, p := range ps[1:] {
		switch {
		case p.typ == plNotify && len(p.body) >= 8 && binary.BigEndian.Uint16(p.body[6:8]) == notifyRUThere:
			ack := append([]byte{}, p.body...)
			binary.BigEndian.PutUint16(ack[6:8], notifyRUThereAck)
			c.sendInfo(k, payload{plNotify, ack})
		case p.typ == plDelete && len(p.body) >= 8 && p.body[4] == protoESP && p.body[5] == 4:
			c.deleted(p.body[8:])
		case p.typ == plDelete && len(p.body) >= 8 && p.body[4] == protoISAKMP:
			c.logf("VPN のサーバーが ISAKMP の SA を消しました")
		}
	}
}

// deleted は、サーバーが消した ESP の SA (サーバーが受ける SPI) と、その対の SA を外す
func (c *Client) deleted(spis []byte) {
	c.mu.Lock()
	for ; len(spis) >= 4; spis = spis[4:] {
		spi := binary.BigEndian.Uint32(spis)
		for i, o := range c.outbound {
			if o.spi == spi {
				c.outbound = append(c.outbound[:i:i], c.outbound[i+1:]...)
				break
			}
		}
		delete(c.inbound, c.pair[spi])
		delete(c.pair, spi)
	}
	empty := len(c.outbound) == 0
	c.mu.Unlock()
	if empty {
		c.fail(errors.New("VPN のサーバーが IPsec の SA を消しました"))
	}
}

// sendInfo は暗号にした Informational を送る (RFC 2409 5.7)
func (c *Client) sendInfo(k *isakmpCrypto, p payload) error {
	msgID := randSPI()
	rest := chain(p)
	ct, _ := k.encrypt(k.phase2IV(msgID), withHash(p.typ, prf(k.h, k.a, be32(msgID), rest), rest))
	return c.writeIKE(header{ispi: c.ispi, rspi: c.rspi, exch: exInfo, next: plHash, flags: flagEnc, msgID: msgID}, ct)
}

// deletePayload は SA を消す知らせ (RFC 2408 3.15)
func deletePayload(proto byte, spis ...[]byte) payload {
	b := append(be32(doiIPsec), proto, byte(len(spis[0])))
	b = binary.BigEndian.AppendUint16(b, uint16(len(spis)))
	for _, s := range spis {
		b = append(b, s...)
	}
	return payload{plDelete, b}
}
