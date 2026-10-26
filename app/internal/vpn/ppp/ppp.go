// Package ppp は PPP (RFC 1661) のクライアント。SSTP と L2TP の中で使う。
// LCP で話をまとめ、MS-CHAPv2 (RFC 2759)、CHAP-MD5 (RFC 1994)、PAP (RFC 1334) で認証し、
// IPCP (RFC 1332、RFC 1877) でアドレスと DNS をもらう。下の層には Address と Control (0xFF 0x03) を付けたフレームを渡す
package ppp

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/asutarisucu/DRAG/app/internal/vpn/mschapv2"
)

// プロトコル番号
const (
	protoIPv4 = 0x0021
	protoIPCP = 0x8021
	protoLCP  = 0xc021
	protoPAP  = 0xc023
	protoCHAP = 0xc223
)

// LCP と IPCP のコード (RFC 1661 5)
const (
	confReq    = 1
	confAck    = 2
	confNak    = 3
	confRej    = 4
	termReq    = 5
	termAck    = 6
	codeRej    = 7
	protoRej   = 8
	echoReq    = 9
	echoReply  = 10
	discardReq = 11
)

// LCP のオプション (RFC 1661 6、RFC 1662)
const (
	optMRU   = 1
	optACCM  = 2
	optAuth  = 3
	optMagic = 5
)

// IPCP のオプション (RFC 1332、RFC 1877)
const (
	optIPAddr = 3
	optDNS1   = 129
	optDNS2   = 131
)

// Config は PPP の認証に使うもの
type Config struct {
	User, Pass string
	// OnAuth は認証が済んだときに呼ぶ。hlak は MS-CHAPv2 の鍵から作った 32 バイト (SSTP の Crypto Binding に使う)。
	// MS-CHAPv2 でなければ 0 が並ぶ
	OnAuth func(hlak []byte) error
	Logf   func(format string, args ...any)
	// RequireMSCHAPv2 は、下の層でサーバーを確かめないとき (L2TP/IPsec の PSK は利用者で共有する) に立てる。
	// PAP と CHAP-MD5 と認証なしを断って MS-CHAPv2 を求め、サーバーの Success の応答でサーバーを確かめる。
	// 立てないと、パスワードの送り方をサーバーが決められ、PAP なら平文で送る
	RequireMSCHAPv2 bool
}

// Result は IPCP でもらったもの
type Result struct {
	Addr netip.Addr
	DNS  []netip.Addr
}

// Conn は PPP の接続1つ
type Conn struct {
	cfg  Config
	send func(frame []byte) error
	in   chan []byte

	mu     sync.Mutex
	magic  uint32
	nextID byte

	lcpOurs, lcpTheirs   bool // 自分の Configure-Request が受け付けられたか、相手のものを受け付けたか
	lcpReq               []byte
	authNaks             int    // 認証の方式を求め直した回数
	auth                 uint16 // 相手が求めた認証のプロトコル。0 なら認証なし
	chapAlg              byte
	chap                 struct{ auth, peer, nt []byte } // MS-CHAPv2 の途中の値
	authDone             bool
	papReq               []byte
	ipcpOurs, ipcpTheirs bool
	ipcpOpts             map[byte][]byte // 自分が求める IPCP のオプションと今の値
	ipcpReq              []byte
	result               Result

	opened chan struct{}
	ipIn   chan []byte
	done   chan struct{}
	once   sync.Once
	err    error
}

// New は PPP を作る。send は下の層へフレームを1つ送る
func New(cfg Config, send func(frame []byte) error) *Conn {
	c := &Conn{cfg: cfg, send: send, in: make(chan []byte, 256), opened: make(chan struct{}), ipIn: make(chan []byte, 512), done: make(chan struct{})}
	c.magic = binary.BigEndian.Uint32(randBytes(4))
	c.ipcpOpts = map[byte][]byte{optIPAddr: make([]byte, 4), optDNS1: make([]byte, 4), optDNS2: make([]byte, 4)}
	return c
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func (c *Conn) logf(format string, args ...any) {
	if c.cfg.Logf != nil {
		c.cfg.Logf(format, args...)
	}
}

// Input は下の層から届いたフレームを1つ渡す
func (c *Conn) Input(frame []byte) {
	select {
	case c.in <- append([]byte{}, frame...):
	case <-c.done:
	default:
	}
}

// Start は LCP、認証、IPCP を済ませ、アドレスをもらうまで待つ
func (c *Conn) Start(ctx context.Context) (Result, error) {
	go c.loop()
	c.mu.Lock()
	c.sendLCPReq()
	c.mu.Unlock()
	select {
	case <-c.opened:
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.result, nil
	case <-c.done:
		return Result{}, c.err
	case <-ctx.Done():
		c.fail(errors.New("PPP の話がまとまりません"))
		return Result{}, c.err
	}
}

func (c *Conn) id() byte {
	c.nextID++
	return c.nextID
}

func (c *Conn) write(proto uint16, data []byte) error {
	b := []byte{0xff, 0x03, byte(proto >> 8), byte(proto)}
	return c.send(append(b, data...))
}

func packet(code, id byte, data []byte) []byte {
	b := []byte{code, id, 0, 0}
	b = append(b, data...)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(b)))
	return b
}

func opt(typ byte, v []byte) []byte { return append([]byte{typ, byte(2 + len(v))}, v...) }

// sendLCPReq は LCP の Configure-Request を送る。オプションは Magic-Number だけ
func (c *Conn) sendLCPReq() {
	c.lcpReq = packet(confReq, c.id(), opt(optMagic, binary.BigEndian.AppendUint32(nil, c.magic)))
	c.write(protoLCP, c.lcpReq)
}

func (c *Conn) sendIPCPReq() {
	var opts []byte
	for _, t := range []byte{optIPAddr, optDNS1, optDNS2} {
		if v, ok := c.ipcpOpts[t]; ok {
			opts = append(opts, opt(t, v)...)
		}
	}
	c.ipcpReq = packet(confReq, c.id(), opts)
	c.write(protoIPCP, c.ipcpReq)
}

func parseOpts(b []byte) [][2][]byte {
	var out [][2][]byte
	for len(b) >= 2 {
		l := int(b[1])
		if l < 2 || l > len(b) {
			break
		}
		out = append(out, [2][]byte{{b[0]}, b[2:l]})
		b = b[l:]
	}
	return out
}

func (c *Conn) loop() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case f := <-c.in:
			c.handleLocked(f)
		case <-t.C:
			// 返事が来ていない要求を送り直す
			c.mu.Lock()
			switch {
			case !c.lcpOurs:
				c.write(protoLCP, c.lcpReq)
			case c.papReq != nil && !c.authDone:
				c.write(protoPAP, c.papReq)
			case c.ipcpReq != nil && !c.ipcpOurs:
				c.write(protoIPCP, c.ipcpReq)
			}
			c.mu.Unlock()
		}
	}
}

// handleLocked はフレームを1つ扱う。サーバーから来たフレームで panic しても、アプリごと落とさずに PPP を終える
func (c *Conn) handleLocked(f []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			c.fail(fmt.Errorf("VPN のサーバーからのフレームを扱えません: %v", r))
		}
	}()
	c.handle(f)
}

// body は、LCP と同じ形のパケットのコード、id、中身。長さの欄が 4 より小さいか、届いた長さを超えるものは捨てる (RFC 1661 5)
func body(p []byte) (code, id byte, data []byte, ok bool) {
	if len(p) < 4 {
		return 0, 0, nil, false
	}
	n := int(binary.BigEndian.Uint16(p[2:4]))
	if n < 4 || n > len(p) {
		return 0, 0, nil, false
	}
	return p[0], p[1], p[4:n], true
}

func (c *Conn) handle(f []byte) {
	if len(f) >= 2 && f[0] == 0xff && f[1] == 0x03 {
		f = f[2:]
	}
	if len(f) < 1 {
		return
	}
	var proto uint16
	if f[0]&1 == 1 { // プロトコルの欄を 1 バイトに縮めたもの
		proto, f = uint16(f[0]), f[1:]
	} else {
		if len(f) < 2 {
			return
		}
		proto, f = binary.BigEndian.Uint16(f[:2]), f[2:]
	}
	switch proto {
	case protoIPv4:
		select {
		case c.ipIn <- append([]byte{}, f...):
		default:
		}
		return
	case protoLCP:
		c.handleLCP(f)
	case protoCHAP:
		c.handleCHAP(f)
	case protoPAP:
		c.handlePAP(f)
	case protoIPCP:
		c.handleIPCP(f)
	default:
		// 知らないプロトコル (IPv6CP や CCP など) は断る
		data := binary.BigEndian.AppendUint16(nil, proto)
		data = append(data, f...)
		c.write(protoLCP, packet(protoRej, c.id(), data))
	}
}

func (c *Conn) handleLCP(p []byte) {
	code, id, data, ok := body(p)
	if !ok {
		return
	}
	switch code {
	case confReq:
		var ack, nak, rej []byte
		auth, alg, other := uint16(0), byte(0), false
		for _, o := range parseOpts(data) {
			typ, v := o[0][0], o[1]
			raw := opt(typ, v)
			switch typ {
			case optMRU, optACCM, optMagic:
				ack = append(ack, raw...)
			case optAuth:
				if len(v) < 2 {
					rej = append(rej, raw...)
					continue
				}
				switch p := binary.BigEndian.Uint16(v[:2]); {
				case p == protoPAP && !c.cfg.RequireMSCHAPv2:
					auth = p
					ack = append(ack, raw...)
				case p == protoCHAP && len(v) == 3 && (v[2] == 0x81 || v[2] == 0x05 && !c.cfg.RequireMSCHAPv2):
					auth, alg = p, v[2]
					ack = append(ack, raw...)
				default:
					other = true
				}
			default:
				rej = append(rej, raw...)
			}
		}
		// 求める認証でなければ、MS-CHAPv2 にしてほしいと返す。認証の無い要求にも、オプションを足して返せる (RFC 1661 5.3)
		if other || auth == 0 && c.cfg.RequireMSCHAPv2 {
			if c.authNaks++; c.authNaks > maxAuthNaks {
				c.fail(errors.New("VPN のサーバーが MS-CHAPv2 での認証に応じません"))
				return
			}
			nak = append(nak, opt(optAuth, []byte{0xc2, 0x23, 0x81})...)
		}
		switch {
		case rej != nil:
			c.write(protoLCP, packet(confRej, id, rej))
		case nak != nil:
			c.write(protoLCP, packet(confNak, id, nak))
		default:
			c.auth, c.chapAlg = auth, alg
			c.write(protoLCP, packet(confAck, id, ack))
			c.lcpTheirs = true
			c.lcpUp()
		}
	case confAck:
		if len(c.lcpReq) > 1 && id == c.lcpReq[1] {
			c.lcpOurs = true
			c.lcpUp()
		}
	case confNak, confRej:
		// Magic-Number を断られたら、オプションなしで出し直す
		c.lcpReq = packet(confReq, c.id(), nil)
		c.write(protoLCP, c.lcpReq)
	case termReq:
		c.write(protoLCP, packet(termAck, id, nil))
		c.fail(errors.New("VPN のサーバーが PPP を終えました"))
	case echoReq:
		c.write(protoLCP, packet(echoReply, id, binary.BigEndian.AppendUint32(nil, c.magic)))
	}
}

// maxAuthNaks は、認証の方式を求め直す回数の上限。RFC 1661 の Max-Failure の既定と同じ
const maxAuthNaks = 5

// lcpUp は、LCP が両方向でまとまったら認証に進む
func (c *Conn) lcpUp() {
	if !c.lcpOurs || !c.lcpTheirs || c.authDone || c.papReq != nil {
		return
	}
	switch c.auth {
	case 0:
		c.authed(nil)
	case protoPAP:
		data := append([]byte{byte(len(c.cfg.User))}, c.cfg.User...)
		data = append(data, byte(len(c.cfg.Pass)))
		data = append(data, c.cfg.Pass...)
		c.papReq = packet(1, c.id(), data)
		c.write(protoPAP, c.papReq)
	}
	// CHAP はサーバーの Challenge を待つ
}

func (c *Conn) handleCHAP(p []byte) {
	code, id, data, ok := body(p)
	if !ok {
		return
	}
	switch code {
	case 1: // Challenge
		if len(data) < 1 || len(data) < 1+int(data[0]) {
			return
		}
		challenge := data[1 : 1+int(data[0])]
		var value []byte
		if c.chapAlg == 0x81 {
			if len(challenge) != 16 {
				return
			}
			c.chap.auth = append([]byte{}, challenge...)
			c.chap.peer = randBytes(16)
			c.chap.nt = mschapv2.NTResponse(c.chap.auth, c.chap.peer, c.cfg.User, c.cfg.Pass)
			value = append(append(append([]byte{}, c.chap.peer...), make([]byte, 8)...), c.chap.nt...)
			value = append(value, 0)
		} else {
			h := md5.New()
			h.Write([]byte{id})
			h.Write([]byte(c.cfg.Pass))
			h.Write(challenge)
			value = h.Sum(nil)
		}
		resp := append([]byte{byte(len(value))}, value...)
		resp = append(resp, c.cfg.User...)
		c.write(protoCHAP, packet(2, id, resp))
	case 3: // Success
		if c.authDone {
			return
		}
		if c.chapAlg == 0x81 {
			if !mschapv2.CheckSuccess(string(data), c.cfg.Pass, c.chap.nt, c.chap.peer, c.chap.auth, c.cfg.User) {
				c.fail(errors.New("VPN のサーバーの応答が合いません。パスワードを知らないサーバーかもしれません"))
				return
			}
			send, recv := mschapv2.ClientKeys(c.cfg.Pass, c.chap.nt)
			c.authed(append(send, recv...))
			return
		}
		c.authed(nil)
	case 4: // Failure
		msg := string(data)
		if bytes.Contains(data, []byte("E=648")) {
			c.fail(errors.New("VPN のパスワードの期限が切れています"))
			return
		}
		c.fail(fmt.Errorf("%w (%s)", ErrAuth, msg))
	}
}

// ErrAuth は、サーバーがユーザー名とパスワードを受け付けなかったとき
var ErrAuth = errors.New("VPN のサーバーがユーザー名かパスワードを受け付けません")

func (c *Conn) handlePAP(p []byte) {
	if len(p) < 4 || c.authDone {
		return
	}
	switch p[0] {
	case 2: // Authenticate-Ack
		c.authed(nil)
	case 3: // Authenticate-Nak
		c.fail(ErrAuth)
	}
}

// authed は認証が済んだら IPCP に進む
func (c *Conn) authed(hlak []byte) {
	c.authDone = true
	if hlak == nil {
		hlak = make([]byte, 32)
	}
	if c.cfg.OnAuth != nil {
		if err := c.cfg.OnAuth(hlak); err != nil {
			c.fail(err)
			return
		}
	}
	c.sendIPCPReq()
}

func (c *Conn) handleIPCP(p []byte) {
	code, id, data, ok := body(p)
	if !ok || !c.authDone {
		return
	}
	switch code {
	case confReq:
		var ack, rej []byte
		for _, o := range parseOpts(data) {
			if o[0][0] == optIPAddr {
				ack = append(ack, opt(optIPAddr, o[1])...)
			} else {
				rej = append(rej, opt(o[0][0], o[1])...)
			}
		}
		if rej != nil {
			c.write(protoIPCP, packet(confRej, id, rej))
			return
		}
		c.write(protoIPCP, packet(confAck, id, ack))
		c.ipcpTheirs = true
		c.ipcpUp()
	case confAck:
		if len(c.ipcpReq) > 1 && id == c.ipcpReq[1] {
			c.ipcpOurs = true
			c.ipcpUp()
		}
	case confNak:
		// サーバーが決めた値にして出し直す
		for _, o := range parseOpts(data) {
			if _, ok := c.ipcpOpts[o[0][0]]; ok && len(o[1]) == 4 {
				c.ipcpOpts[o[0][0]] = append([]byte{}, o[1]...)
			}
		}
		c.sendIPCPReq()
	case confRej:
		for _, o := range parseOpts(data) {
			if o[0][0] == optIPAddr {
				// アドレスをもらえなければ、VPN の中で送れない
				c.fail(errors.New("VPN のサーバーが IP アドレスを渡しません"))
				return
			}
			delete(c.ipcpOpts, o[0][0])
		}
		c.sendIPCPReq()
	case termReq:
		c.write(protoIPCP, packet(termAck, id, nil))
		c.fail(errors.New("VPN のサーバーが IP の設定を終えました"))
	}
}

func (c *Conn) ipcpUp() {
	if !c.ipcpOurs || !c.ipcpTheirs {
		return
	}
	select {
	case <-c.opened:
		return
	default:
	}
	addr, ok := c.ipcpOpts[optIPAddr]
	if !ok || len(addr) != 4 {
		c.fail(errors.New("VPN のサーバーが IP アドレスを渡しません"))
		return
	}
	c.result = Result{Addr: netip.AddrFrom4([4]byte(addr))}
	for _, t := range []byte{optDNS1, optDNS2} {
		if v, ok := c.ipcpOpts[t]; ok && len(v) == 4 {
			if ip := netip.AddrFrom4([4]byte(v)); !ip.IsUnspecified() {
				c.result.DNS = append(c.result.DNS, ip)
			}
		}
	}
	c.logf("PPP がまとまりました。この PC は %s", c.result.Addr)
	close(c.opened)
}

// WriteIP は IP のパケットを送る
func (c *Conn) WriteIP(p []byte) error {
	select {
	case <-c.done:
		return c.err
	default:
	}
	return c.write(protoIPv4, p)
}

// ReadIP は届いた IP のパケットを1つ返す
func (c *Conn) ReadIP() ([]byte, error) {
	select {
	case p := <-c.ipIn:
		return p, nil
	case <-c.done:
		return nil, c.err
	}
}

func (c *Conn) Done() <-chan struct{} { return c.done }

func (c *Conn) Err() error {
	<-c.done
	return c.err
}

func (c *Conn) fail(err error) {
	c.once.Do(func() {
		c.err = err
		close(c.done)
	})
}

// Close は LCP の Terminate-Request を送って終える
func (c *Conn) Close() error {
	c.mu.Lock()
	c.write(protoLCP, packet(termReq, c.id(), nil))
	c.mu.Unlock()
	c.fail(errors.New("PPP を閉じました"))
	return nil
}
