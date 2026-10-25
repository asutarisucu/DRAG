package ikev2

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config は IKEv2 でつなぐ設定
type Config struct {
	Server     string         // host か host:port。port が無ければ 4500
	User, Pass string         // EAP-MSCHAPv2 のユーザー名とパスワード
	CA         *x509.CertPool // サーバーの証明書を確かめる CA。nil なら Windows の信頼する CA
	Logf       func(format string, args ...any)
}

// Pushed はサーバーから受けた設定
type Pushed struct {
	Addrs  []netip.Addr
	DNS    []netip.Addr
	Routes []netip.Prefix
}

var nonESP = []byte{0, 0, 0, 0}

// Client は張った IKEv2 の VPN 1つ
type Client struct {
	cfg  Config
	host string
	conn *net.UDPConn

	mu       sync.Mutex
	ike      *ikeSA
	oldIKE   *ikeSA   // サーバーが鍵を替えたあと、消されるまでの古い SA
	child    *childSA // ESP を送る SA。いちばん新しいもの
	inbound  map[uint32]*childSA
	gen      int
	pending  *pendingReq
	pushed   Pushed
	lastRecv time.Time
	lastSent time.Time

	ipIn      chan []byte
	done      chan struct{}
	closeOnce sync.Once
	err       error
}

type pendingReq struct {
	id    uint32
	plain bool
	ch    chan any // *message (暗号にしたもの) か []byte (IKE_SA_INIT)
}

func (c *Client) logf(format string, args ...any) {
	if c.cfg.Logf != nil {
		c.cfg.Logf(format, args...)
	}
}

// Dial は IKEv2 でサーバーにつなぎ、EAP-MSCHAPv2 で認証して、ESP の SA を作るまで待つ
func Dial(ctx context.Context, cfg Config) (*Client, error) {
	host, port := cfg.Server, "4500"
	if h, p, err := net.SplitHostPort(cfg.Server); err == nil {
		host, port = h, p
	}
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.New(host + " のアドレスが見つかりません")
	}
	ua, err := net.ResolveUDPAddr("udp", net.JoinHostPort(addrs[0], port))
	if err != nil {
		return nil, err
	}
	// 相手へつないだ UDP のソケットだけを使い、待ち受けのソケットは開かない
	conn, err := net.DialUDP("udp", nil, ua)
	if err != nil {
		return nil, err
	}
	c := &Client{cfg: cfg, host: host, conn: conn, inbound: map[uint32]*childSA{}, ipIn: make(chan []byte, 512), done: make(chan struct{}), lastRecv: time.Now(), lastSent: time.Now()}
	go c.recvLoop()
	if err := c.connect(ctx, ua); err != nil {
		c.fail(err)
		return nil, err
	}
	go c.timerLoop()
	return c, nil
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

// ikeProposals は IKE の SA に出す提案。AEAD の組と、CBC と HMAC の組
func ikeProposals(spi []byte) []proposal {
	dh := []transform{{tDH, 19, 0}, {tDH, 31, 0}, {tDH, 14, 0}, {tDH, 20, 0}, {tDH, 21, 0}, {tDH, 2, 0}}
	prf := []transform{{tPRF, prfSHA256, 0}, {tPRF, prfSHA384, 0}, {tPRF, prfSHA512, 0}, {tPRF, prfSHA1, 0}}
	p1 := proposal{num: 1, proto: protoIKE, spi: spi, transforms: append(append([]transform{{tEncr, encrAESGCM16, 256}, {tEncr, encrAESGCM16, 128}, {tEncr, encrChaCha, 0}}, prf...), dh...)}
	p2 := proposal{num: 2, proto: protoIKE, spi: spi, transforms: append(append(append([]transform{{tEncr, encrAESCBC, 256}, {tEncr, encrAESCBC, 128}},
		transform{tInteg, integSHA256, 0}, transform{tInteg, integSHA384, 0}, transform{tInteg, integSHA512, 0}, transform{tInteg, integSHA1, 0}), prf...), dh...)}
	return []proposal{p1, p2}
}

// espProposals は ESP の SA に出す提案。spi は受けるときの SPI
func espProposals(spi uint32) []proposal {
	s := binary.BigEndian.AppendUint32(nil, spi)
	esn := transform{tESN, 0, 0}
	return []proposal{
		{num: 1, proto: protoESP, spi: s, transforms: []transform{{tEncr, encrAESGCM16, 256}, {tEncr, encrAESGCM16, 128}, {tEncr, encrChaCha, 0}, esn}},
		{num: 2, proto: protoESP, spi: s, transforms: []transform{{tEncr, encrAESCBC, 256}, {tEncr, encrAESCBC, 128},
			{tInteg, integSHA256, 0}, {tInteg, integSHA384, 0}, {tInteg, integSHA512, 0}, {tInteg, integSHA1, 0}, esn}},
	}
}

func natHash(spiI, spiR [8]byte, ip net.IP, port int) []byte {
	h := sha1.New()
	h.Write(spiI[:])
	h.Write(spiR[:])
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	h.Write(ip)
	h.Write([]byte{byte(port >> 8), byte(port)})
	return h.Sum(nil)
}

func (c *Client) connect(ctx context.Context, server *net.UDPAddr) error {
	var spiI [8]byte
	copy(spiI[:], randBytes(8))
	ni := randBytes(32)
	group := uint16(19)
	var cookie []byte
	var req, resp []byte
	var respMsg *message
	var dh *dhKey
	for try := 0; ; try++ {
		var err error
		if dh, err = newDH(group); err != nil {
			return err
		}
		m := &message{spiI: spiI, exch: exInit, flags: flagInitiator}
		if cookie != nil {
			m.payloads = append(m.payloads, notify(nCookie, cookie))
		}
		ke := binary.BigEndian.AppendUint16(nil, group)
		m.payloads = append(m.payloads,
			payload{pSA, encodeSA(ikeProposals(nil))},
			payload{pKE, append(append(ke, 0, 0), dh.pub...)},
			payload{pNonce, ni},
			// サーバーに NAT の向こうにいると思わせ、ESP を UDP に包ませる。DRAG は ESP をそのまま送れない
			notify(nNATSource, randBytes(20)),
			notify(nNATDest, natHash(spiI, [8]byte{}, server.IP, server.Port)),
			notify(nFragSupported, nil),
			notify(nSigHashAlgs, []byte{0, 1, 0, 2, 0, 3, 0, 4, 0, 5}),
		)
		req = m.encodePlain()
		r, err := c.request(ctx, 0, req, true)
		if err != nil {
			return err
		}
		resp = r.([]byte)
		hm, next, err := parseHeader(resp)
		if err != nil {
			return err
		}
		if hm.payloads, err = parsePayloads(next, resp[28:]); err != nil {
			return err
		}
		if n := findNotify(hm.payloads, nCookie); n != nil && try < 3 {
			cookie = n.data
			continue
		}
		if n := findNotify(hm.payloads, nInvalidKE); n != nil && len(n.data) == 2 && try < 3 {
			group = binary.BigEndian.Uint16(n.data)
			c.logf("サーバーが DH グループ %d を求めました", group)
			continue
		}
		if n := errorNotify(hm.payloads); n != nil {
			return n
		}
		respMsg = hm
		break
	}
	props, err := parseSA(find(respMsg.payloads, pSA))
	if err != nil || len(props) != 1 {
		return errors.New("サーバーの IKE の SA の返事を読めません")
	}
	s, err := suiteOf(props[0])
	if err != nil {
		return err
	}
	ke := find(respMsg.payloads, pKE)
	nr := find(respMsg.payloads, pNonce)
	if len(ke) < 4 || len(nr) < 16 || s.prf == 0 || s.dh != dh.group {
		return errors.New("サーバーの IKE_SA_INIT の返事が足りません")
	}
	shared, err := dh.priv(ke[4:])
	if err != nil {
		return err
	}
	skeyseed := s.prfFunc(append(append([]byte{}, ni...), nr...), shared)
	sa := deriveIKE(s, skeyseed, ni, nr, spiI, respMsg.spiR, true)
	sa.nextID = 1
	c.mu.Lock()
	c.ike = sa
	c.mu.Unlock()
	c.logf("IKE の SA を作りました (暗号 %d、PRF %d、DH %d)", s.encr, s.prf, s.dh)
	return c.authenticate(ctx, sa, req, resp, ni, nr)
}

func findNotify(ps []payload, typ uint16) *notification {
	for _, n := range notifications(ps) {
		if n.typ == typ {
			return &n
		}
	}
	return nil
}

// idPayload は ID のペイロードの中身。@ を含むユーザー名はメールアドレスの形、ほかは FQDN の形で送る
func idPayload(user string) []byte {
	typ := byte(2) // ID_FQDN
	if strings.Contains(user, "@") {
		typ = 3 // ID_RFC822_ADDR
	}
	return append([]byte{typ, 0, 0, 0}, user...)
}

func tsAll() []byte {
	return []byte{1, 0, 0, 0, 7, 0, 0, 16, 0, 0, 0xff, 0xff, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff}
}

func (c *Client) certReq() []byte {
	// どの CA でもよいので、証明書を送ってほしいと伝える
	return []byte{certX509}
}

func (c *Client) authenticate(ctx context.Context, sa *ikeSA, msg1, msg2, ni, nr []byte) error {
	idi := idPayload(c.cfg.User)
	inSPI := binary.BigEndian.Uint32(randBytes(4)) | 0x01000000
	cp := []byte{1, 0, 0, 0, 0, 1, 0, 0, 0, 3, 0, 0, 0, 13, 0, 0} // CFG_REQUEST: IP4_ADDRESS、IP4_DNS、IP4_SUBNET
	first := &message{exch: exAuth, inner: []payload{
		{pIDi, idi},
		{pCertReq, c.certReq()},
		notify(nInitialContact, nil),
		{pCP, cp},
		{pSA, encodeSA(espProposals(inSPI))},
		{pTSi, tsAll()},
		{pTSr, tsAll()},
	}}
	resp, err := c.send(ctx, sa, first)
	if err != nil {
		return err
	}
	if n := errorNotify(resp.inner); n != nil {
		return n
	}
	// サーバーを証明書と署名で確かめる
	var certs [][]byte
	for _, p := range resp.inner {
		if p.typ == pCert && len(p.body) > 1 && p.body[0] == certX509 {
			certs = append(certs, p.body[1:])
		}
	}
	idr, auth := find(resp.inner, pIDr), find(resp.inner, pAuth)
	if idr == nil || len(auth) < 4 {
		return errors.New("VPN のサーバーが名乗りません (IDr と AUTH がありません)")
	}
	cert, err := verifyServerCert(certs, c.cfg.CA, c.host)
	if err != nil {
		return err
	}
	respSigned := append(append(append([]byte{}, msg2...), ni...), sa.s.prfFunc(sa.pr, idr)...)
	if err := verifySignature(auth[0], auth[4:], cert, respSigned); err != nil {
		return err
	}
	c.logf("サーバーの証明書を確かめました: %s", cert.Subject.CommonName)

	// EAP-MSCHAPv2
	eap := &mschap{user: c.cfg.User, pass: c.cfg.Pass}
	req := find(resp.inner, pEAP)
	for i := 0; ; i++ {
		if req == nil || i > 10 {
			return errors.New("VPN のサーバーが EAP の認証を始めません")
		}
		out, err := eap.respond(req)
		if err != nil {
			return err
		}
		if out == nil {
			break // EAP-Success
		}
		r, err := c.send(ctx, sa, &message{exch: exAuth, inner: []payload{{pEAP, out}}})
		if err != nil {
			return err
		}
		if n := errorNotify(r.inner); n != nil {
			return n
		}
		req = find(r.inner, pEAP)
	}
	msk := eap.msk()
	initSigned := append(append(append([]byte{}, msg1...), nr...), sa.s.prfFunc(sa.pi, idi)...)
	last, err := c.send(ctx, sa, &message{exch: exAuth, inner: []payload{{pAuth, append([]byte{authPSK, 0, 0, 0}, pskAuth(sa.s, msk, initSigned)...)}}})
	if err != nil {
		return err
	}
	if n := errorNotify(last.inner); n != nil {
		return n
	}
	auth = find(last.inner, pAuth)
	if len(auth) < 4 || auth[0] != authPSK || !bytes.Equal(auth[4:], pskAuth(sa.s, msk, respSigned)) {
		return errors.New("VPN のサーバーの EAP のあとの AUTH が合いません")
	}

	// ESP の SA と、もらったアドレス
	props, err := parseSA(find(last.inner, pSA))
	if err != nil || len(props) != 1 || len(props[0].spi) != 4 {
		return errors.New("サーバーの ESP の SA の返事を読めません")
	}
	cs, err := suiteOf(props[0])
	if err != nil {
		return err
	}
	keymat := sa.s.prfPlus(sa.d, append(append([]byte{}, ni...), nr...), childKeyLen(cs))
	child := newChildSA(cs, keymat, inSPI, binary.BigEndian.Uint32(props[0].spi), true, props[0])
	pushed := parseCP(find(last.inner, pCP))
	pushed.Routes = append(pushed.Routes, parseTS(find(last.inner, pTSr))...)
	if len(pushed.Addrs) == 0 {
		return errors.New("VPN のサーバーがこの PC のアドレスを送ってきません")
	}
	c.mu.Lock()
	c.gen++
	child.gen = c.gen
	c.child = child
	c.inbound[inSPI] = child
	c.pushed = pushed
	c.mu.Unlock()
	c.logf("ESP の SA を作りました (暗号 %d)、この PC は %s", cs.encr, pushed.Addrs[0])
	return nil
}

// parseCP は CFG_REPLY を読む
func parseCP(b []byte) Pushed {
	var p Pushed
	if len(b) < 4 || b[0] != 2 {
		return p
	}
	for a := b[4:]; len(a) >= 4; {
		typ, l := binary.BigEndian.Uint16(a[0:2])&0x7fff, int(binary.BigEndian.Uint16(a[2:4]))
		if len(a) < 4+l {
			break
		}
		v := a[4 : 4+l]
		switch {
		case typ == 1 && l == 4:
			p.Addrs = append(p.Addrs, netip.AddrFrom4([4]byte(v)))
		case typ == 3 && l == 4:
			p.DNS = append(p.DNS, netip.AddrFrom4([4]byte(v)))
		case typ == 13 && l == 8:
			ones, _ := net.IPMask(v[4:8]).Size()
			p.Routes = append(p.Routes, netip.PrefixFrom(netip.AddrFrom4([4]byte(v[:4])), ones).Masked())
		}
		a = a[4+l:]
	}
	return p
}

// parseTS は、IPv4 の範囲のうち、プレフィックスで表せるものを返す
func parseTS(b []byte) []netip.Prefix {
	var out []netip.Prefix
	if len(b) < 4 {
		return nil
	}
	n := int(b[0])
	b = b[4:]
	for i := 0; i < n && len(b) >= 4; i++ {
		l := int(binary.BigEndian.Uint16(b[2:4]))
		if l < 8 || l > len(b) {
			break
		}
		if b[0] == 7 && l == 16 {
			start, end := binary.BigEndian.Uint32(b[8:12]), binary.BigEndian.Uint32(b[12:16])
			for bits := 0; bits <= 32; bits++ {
				mask := uint32(0)
				if bits > 0 {
					mask = 0xffffffff << (32 - bits)
				}
				if start&^mask == 0 && end == start|^mask {
					out = append(out, netip.PrefixFrom(netip.AddrFrom4([4]byte(b[8:12])), bits))
					break
				}
			}
		}
		b = b[l:]
	}
	return out
}

// send は要求を暗号にして送り、応答を待つ
func (c *Client) send(ctx context.Context, sa *ikeSA, m *message) (*message, error) {
	c.mu.Lock()
	m.id = sa.nextID
	sa.nextID++
	m.flags = sa.flags(false)
	c.mu.Unlock()
	raw, err := sa.encrypt(m)
	if err != nil {
		return nil, err
	}
	r, err := c.request(ctx, m.id, raw, false)
	if err != nil {
		return nil, err
	}
	return r.(*message), nil
}

// request は要求を送り、応答が来るまで間を延ばしながら送り直す
func (c *Client) request(ctx context.Context, id uint32, raw []byte, plain bool) (any, error) {
	p := &pendingReq{id: id, plain: plain, ch: make(chan any, 1)}
	c.mu.Lock()
	c.pending = p
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.pending == p {
			c.pending = nil
		}
		c.mu.Unlock()
	}()
	wait := 500 * time.Millisecond
	deadline := time.Now().Add(20 * time.Second)
	for {
		if err := c.write(append(append([]byte{}, nonESP...), raw...)); err != nil {
			return nil, err
		}
		t := time.NewTimer(wait)
		select {
		case r := <-p.ch:
			t.Stop()
			return r, nil
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return nil, errors.New("VPN のサーバーから返事がありません")
		case <-c.done:
			t.Stop()
			return nil, c.err
		}
		if time.Now().After(deadline) {
			return nil, errors.New("VPN のサーバーから返事がありません")
		}
		wait = min(wait*2, 5*time.Second)
	}
}

func (c *Client) write(b []byte) error {
	c.mu.Lock()
	c.lastSent = time.Now()
	c.mu.Unlock()
	_, err := c.conn.Write(b)
	return err
}

func (c *Client) recvLoop() {
	buf := make([]byte, 65535)
	for {
		n, err := c.conn.Read(buf)
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			// 相手のポートが閉じていると、Windows では ICMP を受けて読み出しが失敗する。そのまま読み続ける
			select {
			case <-c.done:
				return
			default:
			}
			continue
		}
		b := append([]byte{}, buf[:n]...)
		switch {
		case n == 1 && b[0] == 0xff: // NAT の keepalive
		case n >= 4 && bytes.Equal(b[:4], nonESP):
			c.handleIKE(b[4:])
		case n >= 8:
			c.handleESP(b)
		}
	}
}

// saFor は、SPI が合う IKE の SA を返す
func (c *Client) saFor(spiI, spiR [8]byte) *ikeSA {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, sa := range []*ikeSA{c.ike, c.oldIKE} {
		if sa != nil && sa.spiI == spiI && sa.spiR == spiR {
			return sa
		}
	}
	return nil
}

func (c *Client) handleIKE(raw []byte) {
	h, _, err := parseHeader(raw)
	if err != nil {
		return
	}
	c.mu.Lock()
	p := c.pending
	c.mu.Unlock()
	if h.isResponse() {
		if p == nil || p.id != h.id {
			return
		}
		if p.plain {
			if h.exch == exInit {
				select {
				case p.ch <- raw:
				default:
				}
			}
			return
		}
		sa := c.saFor(h.spiI, h.spiR)
		if sa == nil {
			return
		}
		c.mu.Lock()
		m, err := sa.decrypt(raw)
		c.mu.Unlock()
		if err != nil {
			if !errors.Is(err, errFragPending) {
				c.logf("IKE の応答を捨てました: %v", err)
			}
			return
		}
		c.touch()
		select {
		case p.ch <- m:
		default:
		}
		return
	}
	sa := c.saFor(h.spiI, h.spiR)
	if sa == nil {
		return
	}
	c.handleRequest(sa, h, raw)
}

func (c *Client) touch() {
	c.mu.Lock()
	c.lastRecv = time.Now()
	c.mu.Unlock()
}

// handleRequest はサーバーからの要求 (生きているかの確かめ、SA の削除、鍵の取り替え) に応える
func (c *Client) handleRequest(sa *ikeSA, h *message, raw []byte) {
	c.mu.Lock()
	switch {
	case h.id+1 == sa.nextPeerID && sa.lastResp != nil:
		// 同じ要求の送り直し。前の応答を送り直す
		resp := sa.lastResp
		c.mu.Unlock()
		c.write(append(append([]byte{}, nonESP...), resp...))
		return
	case h.id != sa.nextPeerID:
		c.mu.Unlock()
		return
	}
	m, err := sa.decrypt(raw)
	c.mu.Unlock()
	if err != nil {
		if !errors.Is(err, errFragPending) {
			c.logf("IKE の要求を捨てました: %v", err)
		}
		return
	}
	c.touch()
	var reply []payload
	var after func()
	switch m.exch {
	case exInfo:
		reply, after = c.onInformational(sa, m)
	case exCreateChild:
		reply, after = c.onCreateChild(sa, m)
	default:
		return
	}
	resp := &message{exch: m.exch, id: m.id, flags: sa.flags(true), inner: reply}
	out, err := sa.encrypt(resp)
	if err != nil {
		return
	}
	c.mu.Lock()
	sa.nextPeerID++
	sa.lastResp = out
	c.mu.Unlock()
	c.write(append(append([]byte{}, nonESP...), out...))
	if after != nil {
		after()
	}
}

func (c *Client) onInformational(sa *ikeSA, m *message) ([]payload, func()) {
	var reply []payload
	var after func()
	for _, p := range m.inner {
		if p.typ != pDelete || len(p.body) < 4 {
			continue
		}
		proto, size, n := p.body[0], int(p.body[1]), int(binary.BigEndian.Uint16(p.body[2:4]))
		if proto == protoIKE {
			c.mu.Lock()
			current := sa == c.ike
			if sa == c.oldIKE {
				c.oldIKE = nil
			}
			c.mu.Unlock()
			if current {
				after = func() { c.fail(errors.New("VPN のサーバーが接続を終えました")) }
			}
			continue
		}
		if proto != protoESP || size != 4 {
			continue
		}
		var ours []byte
		for i := 0; i < n && len(p.body) >= 4+4*(i+1); i++ {
			spi := binary.BigEndian.Uint32(p.body[4+4*i:])
			c.mu.Lock()
			for in, ch := range c.inbound {
				if ch.outSPI != spi {
					continue
				}
				delete(c.inbound, in)
				ours = binary.BigEndian.AppendUint32(ours, in)
				if c.child == ch {
					c.child = nil
					for _, other := range c.inbound {
						if c.child == nil || other.gen > c.child.gen {
							c.child = other
						}
					}
				}
			}
			noChild := c.child == nil
			c.mu.Unlock()
			if noChild {
				after = func() { c.fail(errors.New("VPN のサーバーが ESP の SA を消しました")) }
			}
		}
		if len(ours) > 0 {
			d := []byte{protoESP, 4}
			d = binary.BigEndian.AppendUint16(d, uint16(len(ours)/4))
			reply = append(reply, payload{pDelete, append(d, ours...)})
		}
	}
	return reply, after
}

// pickProposal は、相手の提案から DRAG が使えるものを1つ選ぶ。変換は種類ごとに1つにする
func pickProposal(props []proposal, proto byte) (proposal, *suite, bool) {
	for _, p := range props {
		if p.proto != proto {
			continue
		}
		var chosen []transform
		ok := true
		for _, typ := range []byte{tEncr, tPRF, tInteg, tDH, tESN} {
			var pick *transform
			for _, t := range p.transforms {
				if t.typ != typ || pick != nil {
					continue
				}
				if acceptable(t) {
					tt := t
					pick = &tt
				}
			}
			has := false
			for _, t := range p.transforms {
				if t.typ == typ {
					has = true
				}
			}
			if has && pick == nil {
				ok = false
				break
			}
			if pick != nil {
				chosen = append(chosen, *pick)
			}
		}
		if !ok {
			continue
		}
		out := proposal{num: p.num, proto: p.proto, spi: p.spi, transforms: chosen}
		s, err := suiteOf(out)
		if err != nil {
			continue
		}
		return out, s, true
	}
	return proposal{}, nil, false
}

func acceptable(t transform) bool {
	switch t.typ {
	case tEncr:
		return (t.id == encrAESGCM16 || t.id == encrAESCBC) && (t.keyLen == 128 || t.keyLen == 192 || t.keyLen == 256) || t.id == encrChaCha
	case tPRF:
		return prfHash(t.id) != nil
	case tInteg:
		_, n := integHash(t.id)
		return n > 0
	case tDH:
		_, err := newDH(t.id)
		return t.id == 0 || err == nil
	case tESN:
		return t.id == 0
	}
	return false
}

// onCreateChild は、サーバーが始めた鍵の取り替え (ESP の SA と IKE の SA) に応じる
func (c *Client) onCreateChild(sa *ikeSA, m *message) ([]payload, func()) {
	props, err := parseSA(find(m.inner, pSA))
	ni := find(m.inner, pNonce)
	if err != nil || len(props) == 0 || len(ni) < 16 {
		return []payload{notify(nNoProposal, nil)}, nil
	}
	nr := randBytes(32)
	if props[0].proto == protoIKE {
		chosen, s, ok := pickProposal(props, protoIKE)
		kei := find(m.inner, pKE)
		if !ok || s.dh == 0 || len(kei) < 4 || binary.BigEndian.Uint16(kei[:2]) != s.dh {
			return []payload{notify(nNoProposal, nil)}, nil
		}
		dh, _ := newDH(s.dh)
		shared, err := dh.priv(kei[4:])
		if err != nil {
			return []payload{notify(nNoProposal, nil)}, nil
		}
		var spiR, spiI [8]byte
		copy(spiR[:], randBytes(8))
		copy(spiI[:], chosen.spi)
		chosen.spi = spiR[:]
		skeyseed := sa.s.prfFunc(sa.d, append(append(append([]byte{}, shared...), ni...), nr...))
		newSA := deriveIKE(s, skeyseed, ni, nr, spiI, spiR, false)
		ke := binary.BigEndian.AppendUint16(nil, s.dh)
		return []payload{{pSA, encodeSA([]proposal{chosen})}, {pNonce, nr}, {pKE, append(append(ke, 0, 0), dh.pub...)}}, func() {
			c.mu.Lock()
			c.oldIKE, c.ike = c.ike, newSA
			c.mu.Unlock()
			c.logf("サーバーが IKE の SA の鍵を替えました")
		}
	}
	chosen, s, ok := pickProposal(props, protoESP)
	if !ok || len(chosen.spi) != 4 {
		return []payload{notify(nNoProposal, nil)}, nil
	}
	var seed, kePayload []byte
	if s.dh != 0 {
		kei := find(m.inner, pKE)
		if len(kei) < 4 || binary.BigEndian.Uint16(kei[:2]) != s.dh {
			return []payload{notify(nInvalidKE, binary.BigEndian.AppendUint16(nil, s.dh))}, nil
		}
		dh, _ := newDH(s.dh)
		shared, err := dh.priv(kei[4:])
		if err != nil {
			return []payload{notify(nNoProposal, nil)}, nil
		}
		seed = shared
		kePayload = append(append(binary.BigEndian.AppendUint16(nil, s.dh), 0, 0), dh.pub...)
	}
	outSPI := binary.BigEndian.Uint32(chosen.spi)
	inSPI := binary.BigEndian.Uint32(randBytes(4)) | 0x01000000
	chosen.spi = binary.BigEndian.AppendUint32(nil, inSPI)
	seed = append(append(seed, ni...), nr...)
	keymat := sa.s.prfPlus(sa.d, seed, childKeyLen(s))
	child := newChildSA(s, keymat, inSPI, outSPI, false, chosen)
	reply := []payload{{pSA, encodeSA([]proposal{chosen})}, {pNonce, nr}}
	if kePayload != nil {
		reply = append(reply, payload{pKE, kePayload})
	}
	if tsi := find(m.inner, pTSi); tsi != nil {
		reply = append(reply, payload{pTSi, tsi})
	}
	if tsr := find(m.inner, pTSr); tsr != nil {
		reply = append(reply, payload{pTSr, tsr})
	}
	return reply, func() {
		c.mu.Lock()
		c.gen++
		child.gen = c.gen
		c.inbound[inSPI] = child
		c.mu.Unlock()
		c.logf("サーバーが ESP の SA の鍵を替えました")
	}
}

func (c *Client) handleESP(b []byte) {
	spi := binary.BigEndian.Uint32(b[:4])
	c.mu.Lock()
	ch := c.inbound[spi]
	c.mu.Unlock()
	if ch == nil {
		return
	}
	ip, err := ch.open(b)
	if err != nil {
		c.logf("ESP を捨てました: %v", err)
		return
	}
	c.mu.Lock()
	c.lastRecv = time.Now()
	// 新しい SA で届き始めたら、送るのもそちらにする
	if c.child == nil || ch.gen > c.child.gen {
		c.child = ch
	}
	c.mu.Unlock()
	if ip == nil {
		return
	}
	select {
	case c.ipIn <- ip:
	default:
	}
}

// WriteIP は IP のパケットを VPN に送る
func (c *Client) WriteIP(p []byte) error {
	c.mu.Lock()
	ch := c.child
	c.mu.Unlock()
	if ch == nil {
		select {
		case <-c.done:
			return c.err
		default:
			return errors.New("ESP の SA がありません")
		}
	}
	b, err := ch.seal(p)
	if err != nil {
		return err
	}
	return c.write(b)
}

// ReadIP は VPN から届いた IP のパケットを1つ返す
func (c *Client) ReadIP() ([]byte, error) {
	select {
	case p := <-c.ipIn:
		return p, nil
	case <-c.done:
		return nil, c.err
	}
}

func (c *Client) Pushed() Pushed {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pushed
}

func (c *Client) Done() <-chan struct{} { return c.done }

func (c *Client) Err() error {
	<-c.done
	return c.err
}

// timerLoop は、黙っているときに NAT の keepalive を送り、サーバーが生きているかを確かめる
func (c *Client) timerLoop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	var checking atomic.Bool
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
		}
		c.mu.Lock()
		idleOut, idleIn, sa := time.Since(c.lastSent), time.Since(c.lastRecv), c.ike
		c.mu.Unlock()
		if idleOut > 20*time.Second {
			c.write([]byte{0xff})
		}
		if idleIn > 30*time.Second && sa != nil && checking.CompareAndSwap(false, true) {
			go func() {
				defer checking.Store(false)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if _, err := c.send(ctx, sa, &message{exch: exInfo}); err != nil {
					c.fail(errors.New("VPN のサーバーから返事がなくなりました"))
				}
			}()
		}
	}
}

func (c *Client) fail(err error) {
	c.closeOnce.Do(func() {
		c.err = err
		close(c.done)
		c.conn.Close()
	})
}

// Close は接続を終える。サーバーにも IKE の SA を消すと伝える
func (c *Client) Close() error {
	c.mu.Lock()
	sa := c.ike
	c.mu.Unlock()
	select {
	case <-c.done:
		return nil
	default:
	}
	if sa != nil {
		c.mu.Lock()
		m := &message{exch: exInfo, id: sa.nextID, flags: sa.flags(false), inner: []payload{{pDelete, []byte{protoIKE, 0, 0, 0}}}}
		sa.nextID++
		c.mu.Unlock()
		if raw, err := sa.encrypt(m); err == nil {
			c.write(append(append([]byte{}, nonESP...), raw...))
		}
	}
	c.fail(net.ErrClosed)
	return nil
}
