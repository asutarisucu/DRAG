package ovpn

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Options は、.ovpn の外から渡すもの
type Options struct {
	User, Pass string // auth-user-pass のとき。.ovpn に書いてあればそちらを使う
	// NoEKM は、鍵の素を TLS の EKM ではなく古い PRF で作る。試験で古いサーバーの動きを確かめるときだけ使う
	NoEKM bool
	Logf  func(format string, args ...any)
}

// Pushed は、サーバーから受けた設定のうち DRAG が使うもの
type Pushed struct {
	Addrs  []netip.Addr   // VPN の中でのこの PC のアドレス (ifconfig と ifconfig-ipv6)
	DNS    []netip.Addr   // dhcp-option DNS と dns server
	Routes []netip.Prefix // VPN に通す範囲 (route と route-ipv6)
	Cipher string
}

const (
	sendWindow = 6    // 返事を待たずに送る制御パケットの数
	recvWindow = 12   // 並べ直すために取っておく制御パケットの数。OpenVPN の TLS_RELIABLE_N_REC_BUFFERS と同じ
	chunkSize  = 1100 // 制御パケット1つに載せる TLS のバイト数
	transition = 60 * time.Second
)

var pingMagic = []byte{0x2a, 0x18, 0x7b, 0xf3, 0x64, 0x1e, 0xb4, 0xcb, 0x07, 0xed, 0x2d, 0x0a, 0x98, 0x1f, 0xc7, 0x48}
var occMagic = []byte{0x28, 0x7f, 0x34, 0x6b, 0xd4, 0xef, 0x7a, 0x81, 0x2d, 0x56, 0xb8, 0xd3, 0xaf, 0xc5, 0x45, 0x9c}

const occExit = 6

// Client は張った OpenVPN 1つ
type Client struct {
	cfg  *Config
	opts Options
	tr   transport
	wrap *wrapper
	sid  [8]byte

	mu          sync.Mutex
	remoteSid   [8]byte
	gotReset    chan struct{}
	keys        [8]*keyState
	cur         *keyState // データを送る鍵の世代
	peerID      int       // -1 なら DATA_V1
	cipher      string
	ekm         bool
	comp        compMode
	frag        *fragmenter // .ovpn に fragment があるとき
	user, pass  string      // 鍵を替えるときにも送る。auth-token を受けたら pass をそれにする
	pingEvery   time.Duration
	pingRestart time.Duration
	lastSent    time.Time
	lastRecv    time.Time
	pushed      Pushed
	pushCh      chan []string

	ipIn      chan []byte
	done      chan struct{}
	closeOnce sync.Once
	err       error
}

// keyState は鍵の世代 (key_id) 1つ。制御チャネルの再送と並べ直し、その上の TLS、データチャネルの鍵を持つ
type keyState struct {
	id       byte
	sendNext uint32
	unacked  map[uint32]*outPacket
	window   chan struct{}
	recvNext uint32
	recvBuf  map[uint32]*ctrlPacket
	acks     []uint32
	conn     *ctrlConn
	tls      *tls.Conn
	data     *dataKeys
	retire   time.Time // 新しい世代に替えたあと、受けるのに使う期限

	preMaster          []byte
	random1, random2   []byte
	sRandom1, sRandom2 []byte
}

type outPacket struct {
	p      *ctrlPacket
	sentAt time.Time
	wait   time.Duration
}

func (c *Client) logf(format string, args ...any) {
	if c.opts.Logf != nil {
		c.opts.Logf(format, args...)
	}
}

// Dial は OpenVPN のサーバーにつなぎ、TLS と鍵の交換を済ませ、サーバーから設定を受けるまで待つ
func Dial(ctx context.Context, cfg *Config, opts Options) (*Client, error) {
	user, pass := opts.User, opts.Pass
	if cfg.User != "" {
		user, pass = cfg.User, cfg.Pass
	}
	if cfg.AuthUserPass && (user == "" || pass == "") {
		return nil, errors.New("この VPN はユーザー名とパスワードが要ります")
	}
	var lastErr error
	for _, r := range cfg.Remotes {
		tr, err := dialTransport(ctx, r)
		if err != nil {
			lastErr = err
			continue
		}
		c, err := start(ctx, cfg, opts, tr, user, pass)
		if err == nil {
			return c, nil
		}
		lastErr = err
		if ctx.Err() != nil || errors.Is(err, ErrAuth) {
			break
		}
	}
	return nil, lastErr
}

// ErrAuth は、サーバーがユーザー名とパスワードを受け付けなかったとき
var ErrAuth = errors.New("VPN のサーバーがユーザー名かパスワードを受け付けません")

func start(ctx context.Context, cfg *Config, opts Options, tr transport, user, pass string) (*Client, error) {
	w, err := newWrapper(cfg)
	if err != nil {
		tr.close()
		return nil, err
	}
	c := &Client{
		cfg: cfg, opts: opts, tr: tr, wrap: w, peerID: -1, comp: cfg.Comp, user: user, pass: pass,
		gotReset: make(chan struct{}), pushCh: make(chan []string, 4),
		ipIn: make(chan []byte, 512), done: make(chan struct{}),
		lastRecv: time.Now(), lastSent: time.Now(),
	}
	if cfg.Fragment > 0 {
		c.frag = newFragmenter(cfg.Fragment)
	}
	rand.Read(c.sid[:])
	go c.recvLoop()
	go c.timerLoop()
	ok := false
	defer func() {
		if !ok {
			c.Close()
		}
	}()

	ks := c.newKeyState(0)
	if err := c.sendReliable(ks, opHardResetClientV2, nil); err != nil {
		return nil, err
	}
	select {
	case <-c.gotReset:
	case <-ctx.Done():
		return nil, errors.New("VPN のサーバーから返事がありません")
	case <-c.done:
		return nil, c.err
	}
	if err := c.handshake(ctx, ks); err != nil {
		return nil, err
	}
	go c.readControl(ks)

	// 設定を求める。返事が来るまで 3 秒ごとに送り直す
	var opts2 []string
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for opts2 == nil {
		if _, err := ks.tls.Write([]byte("PUSH_REQUEST\x00")); err != nil {
			return nil, err
		}
		select {
		case opts2 = <-c.pushCh:
		case <-t.C:
		case <-ctx.Done():
			return nil, errors.New("VPN のサーバーから設定が届きません")
		case <-c.done:
			return nil, c.err
		}
	}
	if err := c.applyPush(opts2); err != nil {
		return nil, err
	}
	if err := c.makeKeys(ks); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.cur = ks
	c.mu.Unlock()
	ok = true
	return c, nil
}

func (c *Client) newKeyState(id byte) *keyState {
	ks := &keyState{id: id, unacked: map[uint32]*outPacket{}, window: make(chan struct{}, sendWindow), recvBuf: map[uint32]*ctrlPacket{}}
	ks.conn = &ctrlConn{c: c, ks: ks, ready: make(chan struct{}, 1)}
	c.keys[id] = ks
	return ks
}

// handshake は、その世代の TLS をつなぎ、鍵の素を交換する
func (c *Client) handshake(ctx context.Context, ks *keyState) error {
	tc := &tls.Config{InsecureSkipVerify: true, VerifyPeerCertificate: c.verifyServer, MinVersion: tls.VersionTLS12}
	if c.cfg.Cert != nil {
		tc.Certificates = []tls.Certificate{*c.cfg.Cert}
	}
	ks.tls = tls.Client(ks.conn, tc)
	if err := ks.tls.HandshakeContext(ctx); err != nil {
		if c.err != nil {
			return c.err
		}
		return fmt.Errorf("VPN のサーバーと TLS でつなげません: %w", err)
	}
	// 鍵の交換。古い PRF で使う乱数も、いつも付ける
	ks.preMaster, ks.random1, ks.random2 = randBytes(48), randBytes(32), randBytes(32)
	m := []byte{0, 0, 0, 0, 2}
	m = append(m, ks.preMaster...)
	m = append(m, ks.random1...)
	m = append(m, ks.random2...)
	m = appendString(m, c.optionsString())
	c.mu.Lock()
	user, pass := c.user, c.pass
	c.mu.Unlock()
	if c.cfg.AuthUserPass {
		m = appendString(m, user)
		m = appendString(m, pass)
	} else {
		m = append(m, 0, 0, 0, 0)
	}
	m = appendString(m, c.peerInfo())
	if _, err := ks.tls.Write(m); err != nil {
		return err
	}
	buf := make([]byte, 16<<10)
	n, err := ks.tls.Read(buf)
	if err != nil {
		return fmt.Errorf("VPN のサーバーから鍵の交換の返事が来ません: %w", err)
	}
	b := buf[:n]
	if len(b) < 5+64 || b[4]&0x0f != 2 {
		return errors.New("VPN のサーバーの鍵の交換の返事を読めません")
	}
	ks.sRandom1, ks.sRandom2 = append([]byte{}, b[5:37]...), append([]byte{}, b[37:69]...)
	return nil
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

// appendString は、終わりの NUL を含めた長さを 2 バイトで書き、続けて文字列と NUL を書く
func appendString(b []byte, s string) []byte {
	b = binary.BigEndian.AppendUint16(b, uint16(len(s)+1))
	b = append(b, s...)
	return append(b, 0)
}

// optionsString は、サーバーが設定の食い違いを確かめるのに使う文字列。食い違っても警告が出るだけ
func (c *Client) optionsString() string {
	proto := "UDPv4"
	if _, ok := c.tr.(*tcpTransport); ok {
		proto = "TCPv4_CLIENT"
	}
	s := "V4,dev-type tun,link-mtu 1549,tun-mtu 1500,proto " + proto
	if c.cfg.Comp != compNone {
		s += ",comp-lzo"
	}
	return s + ",cipher AES-256-GCM,auth [null-digest],keysize 256,key-method 2,tls-client"
}

func (c *Client) peerInfo() string {
	proto := 2 | 4 // DATA_V2、PUSH_REQUEST を待たずに送ってよい
	if !c.opts.NoEKM {
		proto |= 8 // TLS の EKM で鍵の素を作れる
	}
	return strings.Join([]string{
		"IV_VER=2.6.23", "IV_PLAT=win", "IV_PROTO=" + strconv.Itoa(proto), "IV_NCP=2",
		"IV_CIPHERS=" + strings.Join(supportedCiphers, ":"),
		"IV_LZO_STUB=1", "IV_COMP_STUB=1", "IV_COMP_STUBv2=1", "IV_GUI_VER=DRAG",
	}, "\n") + "\n"
}

// verifyServer はサーバーの証明書を確かめる。OpenVPN v2.6.23 の ssl_verify.c と同じ項目を見る。
// OpenVPN は OpenSSL の ssl_server の用途の確かめに任せているが、DRAG は Go の TLS を使うので自分で確かめる。
// 用途はもっと厳しくし、サーバーの用途 (EKU の serverAuth か、EKU の無い古い証明書なら nsCertType の server) を
// はっきり持つものだけを通す。同じ CA が出したクライアント証明書をサーバーとして通さないため
func (c *Client) verifyServer(raw [][]byte, _ [][]*x509.Certificate) error {
	if len(raw) == 0 {
		return errors.New("VPN のサーバーが証明書を出しません")
	}
	certs := make([]*x509.Certificate, len(raw))
	for i, r := range raw {
		cert, err := x509.ParseCertificate(r)
		if err != nil {
			return err
		}
		certs[i] = cert
	}
	leaf := certs[0]
	chain := certs[:1]
	// CA が無いのは、peer-fingerprint の指紋だけでサーバーを決めるとき (下で確かめる)
	if c.cfg.CA != nil {
		inter := x509.NewCertPool()
		for _, cert := range certs[1:] {
			inter.AddCert(cert)
		}
		chains, err := leaf.Verify(x509.VerifyOptions{Roots: c.cfg.CA, Intermediates: inter, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
		if err != nil {
			return fmt.Errorf("VPN のサーバーの証明書を確かめられません: %w", err)
		}
		chain = chains[0]
		if !serverPurpose(leaf) {
			return errors.New("VPN のサーバーの証明書が、サーバーの用途の証明書ではありません")
		}
		// key usage があるなら、TLS のサーバーに使えるものか
		if hasExt(leaf, oidKeyUsage) && leaf.KeyUsage&(x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment|x509.KeyUsageKeyAgreement) == 0 {
			return errors.New("VPN のサーバーの証明書の key usage が TLS のサーバーに使えるものではありません")
		}
	}
	if c.cfg.RequireKU && !hasExt(leaf, oidKeyUsage) {
		return errors.New("VPN のサーバーの証明書に key usage がありません")
	}
	if len(c.cfg.KeyUsages) > 0 {
		// OpenVPN の並びにする。digitalSignature が 0x80、encipherOnly が 0x01
		var nku uint
		for i := range 8 {
			if leaf.KeyUsage&(1<<i) != 0 {
				nku |= 1 << (7 - i)
			}
		}
		if !slices.ContainsFunc(c.cfg.KeyUsages, func(ku uint) bool { return ku != 0 && nku&ku == ku }) {
			return fmt.Errorf("VPN のサーバーの証明書の key usage %02x が remote-cert-ku と合いません", nku)
		}
	}
	if c.cfg.EKU != "" && !slices.Contains(ekus(leaf), c.cfg.EKU) {
		return fmt.Errorf("VPN のサーバーの証明書の用途に %s がありません", c.cfg.EKU)
	}
	if n := c.cfg.VerifyName; n != "" {
		cn := leaf.Subject.CommonName
		if cn != n && !(c.cfg.VerifyPrefix && strings.HasPrefix(cn, n)) {
			return fmt.Errorf("VPN のサーバーの証明書の名前 %s が verify-x509-name と違います", cn)
		}
	}
	if len(c.cfg.Hashes) > 0 {
		if c.cfg.HashDepth >= len(chain) {
			return errors.New("verify-hash で確かめる CA の証明書がありません")
		}
		der := chain[c.cfg.HashDepth].Raw
		var fp []byte
		if len(c.cfg.Hashes[0]) == sha1.Size {
			h := sha1.Sum(der)
			fp = h[:]
		} else {
			h := sha256.Sum256(der)
			fp = h[:]
		}
		if !slices.ContainsFunc(c.cfg.Hashes, func(h []byte) bool { return subtle.ConstantTimeCompare(h, fp) == 1 }) {
			return fmt.Errorf("VPN のサーバーの証明書の指紋 %X が合いません", fp)
		}
	}
	if len(c.cfg.CRLs) > 0 {
		if err := checkCRL(chain, c.cfg.CRLs, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

var (
	oidKeyUsage    = asn1.ObjectIdentifier{2, 5, 29, 15}
	oidExtKeyUsage = asn1.ObjectIdentifier{2, 5, 29, 37}
	oidNSCertType  = asn1.ObjectIdentifier{2, 16, 840, 1, 113730, 1, 1}
)

func hasExt(cert *x509.Certificate, oid asn1.ObjectIdentifier) bool {
	return slices.ContainsFunc(cert.Extensions, func(e pkix.Extension) bool { return e.Id.Equal(oid) })
}

// ekus は証明書の拡張鍵用途の OID。拡張が無ければ nil
func ekus(cert *x509.Certificate) []string {
	for _, e := range cert.Extensions {
		if e.Id.Equal(oidExtKeyUsage) {
			var oids []asn1.ObjectIdentifier
			if _, err := asn1.Unmarshal(e.Value, &oids); err != nil {
				return nil
			}
			out := make([]string, len(oids))
			for i, o := range oids {
				out[i] = o.String()
			}
			return out
		}
	}
	return nil
}

// serverPurpose は、証明書がサーバーの用途をはっきり持つか。
// 拡張鍵用途があれば serverAuth を、無ければ Netscape の証明書の種類 (nsCertType) の server を見る
func serverPurpose(cert *x509.Certificate) bool {
	if hasExt(cert, oidExtKeyUsage) {
		return slices.Contains(ekus(cert), oidServerAuth)
	}
	for _, e := range cert.Extensions {
		if e.Id.Equal(oidNSCertType) {
			var bits asn1.BitString
			if _, err := asn1.Unmarshal(e.Value, &bits); err != nil {
				return false
			}
			return bits.At(1) == 1 // SSL server
		}
	}
	return false
}

// checkCRL は、chain のどの証明書も、発行した CA の CRL で失効していないかを確かめる。
// OpenVPN は OpenSSL の X509_V_FLAG_CRL_CHECK_ALL を立てるので、CRL の無い CA があれば断る
func checkCRL(chain []*x509.Certificate, crls []*x509.RevocationList, now time.Time) error {
	for i, cert := range chain {
		issuer := cert
		if i+1 < len(chain) {
			issuer = chain[i+1]
		}
		var rl *x509.RevocationList
		for _, r := range crls {
			if bytes.Equal(r.RawIssuer, cert.RawIssuer) && r.CheckSignatureFrom(issuer) == nil {
				rl = r
				break
			}
		}
		if rl == nil {
			return fmt.Errorf("%s を発行した CA の CRL が <crl-verify> にありません", cert.Subject.CommonName)
		}
		if !rl.NextUpdate.IsZero() && now.After(rl.NextUpdate) {
			return fmt.Errorf("%s の CRL の期限が切れています", cert.Issuer.CommonName)
		}
		for _, r := range rl.RevokedCertificateEntries {
			if r.SerialNumber.Cmp(cert.SerialNumber) == 0 {
				return fmt.Errorf("VPN のサーバーの証明書 %s は失効しています", cert.Subject.CommonName)
			}
		}
	}
	return nil
}

// makeKeys は、その世代のデータチャネルの鍵を作る
func (c *Client) makeKeys(ks *keyState) error {
	var km []byte
	if c.ekm {
		var err error
		st := ks.tls.ConnectionState()
		if km, err = st.ExportKeyingMaterial("EXPORTER-OpenVPN-datakeys", nil, 256); err != nil {
			return err
		}
	} else {
		c.mu.Lock()
		remote := c.remoteSid
		c.mu.Unlock()
		master := prf10(ks.preMaster, concat([]byte("OpenVPN master secret"), ks.random1, ks.sRandom1), 48)
		km = prf10(master, concat([]byte("OpenVPN key expansion"), ks.random2, ks.sRandom2, c.sid[:], remote[:]), 256)
	}
	k, err := newDataKeys(c.cipher, c.cfg.Auth, km)
	if err != nil {
		return err
	}
	c.mu.Lock()
	ks.data = k
	c.mu.Unlock()
	return nil
}

func concat(parts ...[]byte) []byte {
	var b []byte
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}

// prf10 は TLS 1.0 の PRF。seed にはラベルを含める
func prf10(secret, seed []byte, n int) []byte {
	half := (len(secret) + 1) / 2
	a := pHash(md5.New, secret[:half], seed, n)
	b := pHash(sha1.New, secret[len(secret)-half:], seed, n)
	for i := range a {
		a[i] ^= b[i]
	}
	return a
}

func pHash(h func() hash.Hash, secret, seed []byte, n int) []byte {
	var out []byte
	a := seed
	for len(out) < n {
		m := hmac.New(h, secret)
		m.Write(a)
		a = m.Sum(nil)
		m = hmac.New(h, secret)
		m.Write(a)
		m.Write(seed)
		out = m.Sum(out)
	}
	return out[:n]
}

// applyPush はサーバーから受けた設定を使う
func (c *Client) applyPush(opts []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cipher := ""
	for _, o := range opts {
		f := strings.Fields(o)
		if len(f) == 0 {
			continue
		}
		// auth-token はパスワードの代わりなので、ログに書かない
		if !strings.HasPrefix(f[0], "auth-token") {
			c.logf("サーバーの設定: %s", o)
		}
		arg := func(i int) string {
			if i < len(f) {
				return f[i]
			}
			return ""
		}
		switch f[0] {
		case "ifconfig":
			if ip, err := netip.ParseAddr(arg(1)); err == nil {
				c.pushed.Addrs = append(c.pushed.Addrs, ip)
			}
		case "ifconfig-ipv6":
			if p, err := netip.ParsePrefix(arg(1)); err == nil {
				c.pushed.Addrs = append(c.pushed.Addrs, p.Addr())
			}
		case "route":
			ip, err1 := netip.ParseAddr(arg(1))
			bits := 32
			if m := net.ParseIP(arg(2)).To4(); m != nil {
				bits, _ = net.IPMask(m).Size()
			}
			if err1 == nil {
				c.pushed.Routes = append(c.pushed.Routes, netip.PrefixFrom(ip, bits).Masked())
			}
		case "route-ipv6":
			if p, err := netip.ParsePrefix(arg(1)); err == nil {
				c.pushed.Routes = append(c.pushed.Routes, p.Masked())
			}
		case "dhcp-option":
			if arg(1) == "DNS" || arg(1) == "DNS6" {
				if ip, err := netip.ParseAddr(arg(2)); err == nil {
					c.pushed.DNS = append(c.pushed.DNS, ip)
				}
			}
		case "dns":
			// dns server 0 address 10.8.0.1 [...]
			if arg(1) == "server" && arg(3) == "address" {
				for _, a := range f[4:] {
					if ap, err := netip.ParseAddrPort(a); err == nil {
						c.pushed.DNS = append(c.pushed.DNS, ap.Addr())
					} else if ip, err := netip.ParseAddr(strings.Trim(a, "[]")); err == nil {
						c.pushed.DNS = append(c.pushed.DNS, ip)
					}
				}
			}
		case "cipher":
			cipher = strings.ToUpper(arg(1))
		case "peer-id":
			c.peerID, _ = strconv.Atoi(arg(1))
		case "key-derivation":
			c.ekm = c.ekm || arg(1) == "tls-ekm"
		case "protocol-flags":
			for _, fl := range f[1:] {
				if fl == "tls-ekm" {
					c.ekm = true
				}
			}
		case "auth-token":
			c.pass = arg(1)
		case "comp-lzo":
			c.comp = compLZO
		case "compress":
			c.comp = compressMode(arg(1))
		case "ping":
			n, _ := strconv.Atoi(arg(1))
			c.pingEvery = time.Duration(n) * time.Second
		case "ping-restart":
			n, _ := strconv.Atoi(arg(1))
			c.pingRestart = time.Duration(n) * time.Second
		}
	}
	if c.opts.NoEKM {
		c.ekm = false
	}
	if cipher == "" {
		cipher = c.cfg.Cipher
	}
	c.cipher = cipher
	c.pushed.Cipher = cipher
	if len(c.pushed.Addrs) == 0 {
		return errors.New("VPN のサーバーがこの PC のアドレス (ifconfig) を送ってきません")
	}
	return nil
}

// Pushed はサーバーから受けた設定
func (c *Client) Pushed() Pushed {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pushed
}

// readControl は、TLS の上の文字のメッセージを読む。1つの TLS のレコードが1つのメッセージ
func (c *Client) readControl(ks *keyState) {
	buf := make([]byte, 16<<10)
	var push []string
	for {
		n, err := ks.tls.Read(buf)
		if err != nil {
			return
		}
		msg := strings.TrimRight(string(buf[:n]), "\x00")
		c.logf("制御: %s", strings.SplitN(msg, ",", 2)[0])
		switch {
		case strings.HasPrefix(msg, "PUSH_REPLY"):
			opts := strings.Split(msg, ",")[1:]
			push = append(push, opts...)
			more := false
			for _, o := range opts {
				if o == "push-continuation 2" {
					more = true
				}
			}
			if !more {
				select {
				case c.pushCh <- push:
				default:
				}
				push = nil
			}
		case strings.HasPrefix(msg, "AUTH_FAILED"):
			reason := strings.TrimPrefix(strings.TrimPrefix(msg, "AUTH_FAILED"), ",")
			err := ErrAuth
			if reason != "" {
				err = fmt.Errorf("%w (%s)", ErrAuth, reason)
			}
			c.fail(err)
			return
		case strings.HasPrefix(msg, "RESTART"), strings.HasPrefix(msg, "HALT"), msg == "EXIT":
			c.fail(fmt.Errorf("VPN のサーバーが接続を終えました: %s", msg))
			return
		}
	}
}

// sendReliable は、返事 (ACK) が来るまで送り直す制御パケットを送る。返事を待つものが多いときは空くまで待つ
func (c *Client) sendReliable(ks *keyState, op byte, payload []byte) error {
	select {
	case ks.window <- struct{}{}:
	case <-c.done:
		return c.err
	}
	c.mu.Lock()
	p := &ctrlPacket{op: op, keyID: ks.id, sid: c.sid, id: ks.sendNext, payload: payload}
	ks.sendNext++
	c.takeAcks(ks, p, 4)
	ks.unacked[p.id] = &outPacket{p: p, sentAt: time.Now(), wait: 2 * time.Second}
	c.mu.Unlock()
	return c.transmit(p)
}

// takeAcks は、まだ返していない ACK を p に載せる
func (c *Client) takeAcks(ks *keyState, p *ctrlPacket, max int) {
	n := min(len(ks.acks), max)
	p.acks = append([]uint32{}, ks.acks[:n]...)
	ks.acks = ks.acks[n:]
	p.peerSid = c.remoteSid
}

func (c *Client) transmit(p *ctrlPacket) error {
	return c.tr.send(c.wrap.wrap(p))
}

// sendAcks は、返していない ACK を ACK_V1 で返す
func (c *Client) sendAcks(ks *keyState) {
	for {
		c.mu.Lock()
		if len(ks.acks) == 0 {
			c.mu.Unlock()
			return
		}
		p := &ctrlPacket{op: opAck, keyID: ks.id, sid: c.sid}
		c.takeAcks(ks, p, 8)
		c.mu.Unlock()
		c.transmit(p)
	}
}

func (c *Client) recvLoop() {
	for {
		b, err := c.tr.recv()
		if err != nil {
			c.fail(fmt.Errorf("VPN のサーバーとの通信が切れました: %w", err))
			return
		}
		if len(b) == 0 {
			continue
		}
		op := b[0] >> 3
		switch {
		case op == opDataV1 || op == opDataV2:
			c.handleData(b)
		case isControl(op):
			p, err := c.wrap.unwrap(b)
			if err != nil {
				c.logf("制御チャネルのパケットを捨てました: %v", err)
				continue
			}
			c.handleControl(p)
		}
	}
}

func (c *Client) handleControl(p *ctrlPacket) {
	c.mu.Lock()
	if p.op == opHardResetServerV2 && p.keyID == 0 {
		select {
		case <-c.gotReset:
		default:
			c.remoteSid = p.sid
			close(c.gotReset)
		}
	}
	if p.sid != c.remoteSid {
		c.mu.Unlock()
		return
	}
	c.lastRecv = time.Now()
	ks := c.keys[p.keyID]
	newSoft := false
	if p.op == opSoftReset && ks == nil {
		// サーバーが鍵を替え始めた。新しい世代を作る
		ks = c.newKeyState(p.keyID)
		newSoft = true
	}
	if ks == nil {
		c.mu.Unlock()
		return
	}
	for _, a := range p.acks {
		if _, ok := ks.unacked[a]; ok {
			delete(ks.unacked, a)
			<-ks.window
		}
	}
	if p.op != opAck {
		// 窓より先のものは取っておかずに捨てる。サーバーは返事 (ACK) が来ないので送り直す。
		// tls-auth も tls-crypt も無ければ、経路の途中の誰でも制御パケットを送れるので、取っておく数を決めておく
		if p.id >= ks.recvNext && p.id-ks.recvNext < recvWindow {
			ks.acks = append(ks.acks, p.id)
			if _, dup := ks.recvBuf[p.id]; !dup {
				ks.recvBuf[p.id] = p
			}
		} else if p.id < ks.recvNext {
			// 受けたのに ACK が届かず、送り直されたもの
			ks.acks = append(ks.acks, p.id)
		}
		for {
			q, ok := ks.recvBuf[ks.recvNext]
			if !ok {
				break
			}
			delete(ks.recvBuf, ks.recvNext)
			ks.recvNext++
			if q.op == opControl {
				ks.conn.push(q.payload)
			}
		}
	}
	c.mu.Unlock()

	if newSoft {
		c.logf("鍵の世代 %d に替えます", ks.id)
		go c.renegotiate(ks)
		return
	}
	c.sendAcks(ks)
}

// renegotiate は、サーバーが始めた鍵の取り替えに応じる。同じ設定のまま、新しい世代の TLS と鍵を作る
func (c *Client) renegotiate(ks *keyState) {
	// 失敗した世代は残さない。残すと、サーバーがあとでその key_id に替えようとしても新しく始められない
	drop := func(err error) {
		c.logf("鍵の取り替えに失敗しました: %v", err)
		c.mu.Lock()
		if c.keys[ks.id] == ks {
			c.keys[ks.id] = nil
		}
		c.mu.Unlock()
		ks.conn.Close()
	}
	if err := c.sendReliable(ks, opSoftReset, nil); err != nil {
		drop(err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), transition)
	defer cancel()
	if err := c.handshake(ctx, ks); err != nil {
		drop(err)
		return
	}
	if err := c.makeKeys(ks); err != nil {
		drop(err)
		return
	}
	go c.readControl(ks)
	c.mu.Lock()
	if old := c.cur; old != nil && old != ks {
		old.retire = time.Now().Add(transition)
	}
	c.cur = ks
	c.mu.Unlock()
}

func (c *Client) timerLoop() {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
		}
		now := time.Now()
		var resend []*ctrlPacket
		c.mu.Lock()
		for i, ks := range c.keys {
			if ks == nil {
				continue
			}
			if !ks.retire.IsZero() && now.After(ks.retire) {
				ks.conn.Close()
				c.keys[i] = nil
				continue
			}
			for _, o := range ks.unacked {
				if now.Sub(o.sentAt) >= o.wait {
					o.sentAt, o.wait = now, min(o.wait*2, 30*time.Second)
					// 送り直すときも、そのとき返していない ACK を載せる
					c.takeAcks(ks, o.p, 4)
					resend = append(resend, o.p)
				}
			}
		}
		cur, pingDue := c.cur, c.pingEvery > 0 && now.Sub(c.lastSent) >= c.pingEvery
		dead := c.pingRestart > 0 && now.Sub(c.lastRecv) > c.pingRestart
		c.mu.Unlock()
		for _, p := range resend {
			c.transmit(p)
		}
		if dead {
			c.fail(errors.New("VPN のサーバーから返事がなくなりました"))
			return
		}
		if cur != nil && cur.data != nil && pingDue {
			c.sendPayload(pingMagic)
		}
	}
}

// WriteIP は IP のパケットを VPN に送る
func (c *Client) WriteIP(p []byte) error {
	select {
	case <-c.done:
		return c.err
	default:
	}
	return c.sendPayload(p)
}

// sendPayload は、圧縮の印を付け、fragment があれば分けて、データのパケットで送る (OpenVPN の forward.c の順)
func (c *Client) sendPayload(p []byte) error {
	framed := c.frame(p)
	if c.frag == nil {
		return c.sendData(framed)
	}
	parts := c.frag.split(framed)
	if parts == nil {
		return errors.New("パケットが大きすぎて fragment で分けられません")
	}
	for _, part := range parts {
		if err := c.sendData(part); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) sendData(payload []byte) error {
	c.mu.Lock()
	ks, peer := c.cur, c.peerID
	c.lastSent = time.Now()
	c.mu.Unlock()
	if ks == nil || ks.data == nil {
		return errors.New("VPN のデータチャネルがまだありません")
	}
	head := []byte{opDataV1<<3 | ks.id}
	if peer >= 0 {
		head = []byte{opDataV2<<3 | ks.id, byte(peer >> 16), byte(peer >> 8), byte(peer)}
	}
	b, err := ks.data.seal(head, payload)
	if err != nil {
		c.fail(err)
		return err
	}
	return c.tr.send(b)
}

// frame は圧縮の印を付ける。DRAG は圧縮しない
func (c *Client) frame(p []byte) []byte {
	if len(p) == 0 {
		return p
	}
	c.mu.Lock()
	mode := c.comp
	c.mu.Unlock()
	switch mode {
	case compLZO:
		return append([]byte{0xfa}, p...)
	case compSwap:
		out := append([]byte{0xfb}, p[1:]...)
		return append(out, p[0])
	case compV2:
		if p[0] == 0x50 {
			return append([]byte{0x50, 0}, p...)
		}
	}
	return p
}

// unframe は圧縮の印を外し、サーバーが圧縮したものは戻す。戻せないものは捨てる
func (c *Client) unframe(p []byte) []byte {
	if len(p) == 0 {
		return nil
	}
	c.mu.Lock()
	mode := c.comp
	c.mu.Unlock()
	var out []byte
	var err error
	switch mode {
	case compLZO:
		switch p[0] {
		case 0xfa:
			return p[1:]
		case lzoCompressByte:
			out, err = lzo1xDecompress(p[1:])
		default:
			return nil
		}
	case compSwap:
		if len(p) < 2 {
			return nil
		}
		// 先頭の印の場所に、末尾へ動かした元の先頭を戻す
		body := append([]byte{p[len(p)-1]}, p[1:len(p)-1]...)
		switch p[0] {
		case 0xfb:
			return body
		case lz4CompressByte:
			out, err = lz4Decompress(body)
		default:
			return nil
		}
	case compV2:
		if p[0] != 0x50 {
			return p
		}
		if len(p) < 2 {
			return nil
		}
		switch p[1] {
		case 0:
			return p[2:]
		case v2LZ4Byte:
			out, err = lz4Decompress(p[2:])
		case v2LZOByte:
			out, err = lzo1xDecompress(p[2:])
		default:
			return nil
		}
	default:
		return p
	}
	if err != nil {
		return nil
	}
	return out
}

func (c *Client) handleData(b []byte) {
	op, id := b[0]>>3, b[0]&7
	hl := 1
	if op == opDataV2 {
		hl = 4
	}
	if len(b) < hl {
		return
	}
	c.mu.Lock()
	ks := c.keys[id]
	c.mu.Unlock()
	if ks == nil || ks.data == nil {
		return
	}
	plain, err := ks.data.open(b[:hl], b[hl:])
	if err != nil {
		c.logf("データのパケットを捨てました: %v", err)
		return
	}
	c.mu.Lock()
	c.lastRecv = time.Now()
	c.mu.Unlock()
	// 分かれて届いたものをつなぎ、圧縮の印を外してから、ping と OCC を見分ける (OpenVPN の forward.c の順)
	if c.frag != nil {
		if plain = c.frag.join(plain); plain == nil {
			return
		}
	}
	ip := c.unframe(plain)
	if ip == nil {
		c.logf("圧縮の印を読めないパケットを捨てました")
		return
	}
	if bytes.Equal(ip, pingMagic) || bytes.HasPrefix(ip, occMagic) {
		return
	}
	select {
	case c.ipIn <- ip:
	default:
	}
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

// Done は接続が終わると閉じる。Err で理由を返す
func (c *Client) Done() <-chan struct{} { return c.done }

func (c *Client) Err() error {
	<-c.done
	return c.err
}

func (c *Client) fail(err error) {
	c.closeOnce.Do(func() {
		c.err = err
		close(c.done)
		c.tr.close()
		c.mu.Lock()
		for _, ks := range c.keys {
			if ks != nil {
				ks.conn.Close()
			}
		}
		c.mu.Unlock()
	})
}

// Close は接続を終える。サーバーにも終わったことを伝える
func (c *Client) Close() error {
	c.mu.Lock()
	ready := c.cur != nil && c.cur.data != nil
	c.mu.Unlock()
	if ready {
		select {
		case <-c.done:
		default:
			c.sendPayload(append(append([]byte{}, occMagic...), occExit))
		}
	}
	c.fail(net.ErrClosed)
	return nil
}

// ctrlConn は、1つの世代の制御チャネルを crypto/tls に渡す net.Conn にしたもの
type ctrlConn struct {
	c      *Client
	ks     *keyState
	mu     sync.Mutex
	buf    []byte
	closed bool
	ready  chan struct{}
}

func (cc *ctrlConn) push(b []byte) {
	cc.mu.Lock()
	cc.buf = append(cc.buf, b...)
	cc.mu.Unlock()
	select {
	case cc.ready <- struct{}{}:
	default:
	}
}

func (cc *ctrlConn) Read(b []byte) (int, error) {
	for {
		cc.mu.Lock()
		if len(cc.buf) > 0 {
			n := copy(b, cc.buf)
			cc.buf = cc.buf[n:]
			cc.mu.Unlock()
			return n, nil
		}
		if cc.closed {
			cc.mu.Unlock()
			return 0, io.EOF
		}
		cc.mu.Unlock()
		<-cc.ready
	}
}

func (cc *ctrlConn) Write(b []byte) (int, error) {
	for off := 0; off < len(b); off += chunkSize {
		end := min(off+chunkSize, len(b))
		if err := cc.c.sendReliable(cc.ks, opControl, append([]byte{}, b[off:end]...)); err != nil {
			return off, err
		}
	}
	return len(b), nil
}

func (cc *ctrlConn) Close() error {
	cc.mu.Lock()
	cc.closed = true
	cc.mu.Unlock()
	select {
	case cc.ready <- struct{}{}:
	default:
	}
	return nil
}

func (cc *ctrlConn) LocalAddr() net.Addr                { return dummyAddr{} }
func (cc *ctrlConn) RemoteAddr() net.Addr               { return dummyAddr{} }
func (cc *ctrlConn) SetDeadline(t time.Time) error      { return nil }
func (cc *ctrlConn) SetReadDeadline(t time.Time) error  { return nil }
func (cc *ctrlConn) SetWriteDeadline(t time.Time) error { return nil }

type dummyAddr struct{}

func (dummyAddr) Network() string { return "openvpn" }
func (dummyAddr) String() string  { return "openvpn" }
