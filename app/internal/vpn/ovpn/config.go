// Package ovpn は OpenVPN のクライアント。TLS モードで、サーバーとのやり取りと
// データチャネルの暗号を Go で行い、IP のパケットを読み書きする口を出す。
// 仕様は OpenVPN/openvpn-rfc の openvpn-wire-protocol.xml。仕様に無い部分 (圧縮の印や ping の中身) は
// OpenVPN v2.6.23 の挙動に合わせている。コードは写していない
package ovpn

import (
	"bufio"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Config は .ovpn から読んだクライアントの設定。Windows の側を変える項目 (route や dhcp-option の適用) は使わない
type Config struct {
	Remotes      []Remote
	CA           *x509.CertPool
	Cert         *tls.Certificate       // クライアント証明書。ユーザー名とパスワードだけで入るときは nil
	VerifyName   string                 // verify-x509-name の値
	VerifyPrefix bool                   // verify-x509-name の name-prefix
	EKU          string                 // remote-cert-eku の OID。remote-cert-tls server ならサーバーの用途
	RequireKU    bool                   // remote-cert-tls か remote-cert-ku。サーバーの証明書に key usage が無ければ断る
	KeyUsages    []uint                 // remote-cert-ku の値。OpenVPN と同じく digitalSignature を 0x80 とする並びで、どれか1つのビットをすべて持てばよい
	Hashes       [][]byte               // verify-hash か peer-fingerprint の指紋
	HashDepth    int                    // 指紋を当てる証明書。0 はサーバーの証明書 (peer-fingerprint)、1 はそれに署名した CA (verify-hash)
	CRLs         []*x509.RevocationList // <crl-verify>
	TLSAuth      *staticKey
	TLSAuthDir   int // key-direction。-1 なら向きを分けない
	TLSCrypt     *staticKey
	Auth         string // tls-auth と CBC の HMAC のハッシュ。既定は SHA1
	Cipher       string // サーバーが cipher を送ってこないときに使う
	DataCiphers  []string
	AuthUserPass bool
	User, Pass   string // <auth-user-pass> に書いてあったとき
	Comp         compMode
	Fragment     int // fragment の値。UDP の中身の大きさの上限で、0 なら分けない
}

type Remote struct {
	Host, Port string
	TCP        bool
}

// compMode は圧縮の枠の付け方。DRAG は圧縮して送らず、印だけを付ける
type compMode int

const (
	compNone compMode = iota
	compLZO           // comp-lzo と compress lzo。先頭に 0xFA
	compSwap          // compress stub と compress lz4。先頭の1バイトを末尾へ動かし、先頭に 0xFB
	compV2            // compress stub-v2 と compress lz4-v2。先頭が 0x50 のときだけ印を足す
)

// staticKey は OpenVPN Static key V1 の 2048 ビットの鍵。[0] と [1] がそれぞれ暗号の 64 バイトと HMAC の 64 バイト
type staticKey [2]struct{ cipher, hmac [64]byte }

// Parse は .ovpn を読む。対応していない作りの設定は、項目の名前を出して断る
func Parse(text string) (*Config, error) {
	c := &Config{Auth: "SHA1", Cipher: "BF-CBC", TLSAuthDir: -1}
	blocks := map[string]string{}
	var lines [][]string
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		// <ca> ... </ca> のような埋め込み
		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") && !strings.HasPrefix(line, "</") {
			tag := line[1 : len(line)-1]
			var b strings.Builder
			for sc.Scan() {
				l := strings.TrimSpace(sc.Text())
				if l == "</"+tag+">" {
					break
				}
				b.WriteString(l + "\n")
			}
			blocks[tag] = b.String()
			continue
		}
		lines = append(lines, fields(line))
	}
	port, tcp := "1194", false
	for _, f := range lines {
		switch f[0] {
		case "port", "rport":
			if len(f) > 1 {
				port = f[1]
			}
		case "proto":
			if len(f) > 1 {
				tcp = strings.HasPrefix(f[1], "tcp")
			}
		}
	}
	for _, f := range lines {
		arg := func(i int) string {
			if i < len(f) {
				return f[i]
			}
			return ""
		}
		switch f[0] {
		case "remote":
			r := Remote{Host: arg(1), Port: port, TCP: tcp}
			if arg(2) != "" {
				r.Port = arg(2)
			}
			if arg(3) != "" {
				r.TCP = strings.HasPrefix(arg(3), "tcp")
			}
			c.Remotes = append(c.Remotes, r)
		case "dev":
			if strings.HasPrefix(arg(1), "tap") {
				return nil, errors.New("dev tap (イーサネットのフレームを通す形) には対応していません")
			}
		case "dev-type":
			if arg(1) == "tap" {
				return nil, errors.New("dev-type tap には対応していません")
			}
		case "secret":
			return nil, errors.New("secret (静的な鍵だけで張る古い形) には対応していません")
		case "tls-crypt-v2", "pkcs12":
			return nil, fmt.Errorf("%s にはまだ対応していません", f[0])
		case "remote-cert-tls":
			switch arg(1) {
			case "server":
				c.EKU = oidServerAuth
			case "client":
				c.EKU = oidClientAuth
			default:
				return nil, errors.New("remote-cert-tls は server か client です")
			}
			c.RequireKU = true
		case "remote-cert-eku":
			if c.EKU = ekuOID(arg(1)); c.EKU == "" {
				return nil, fmt.Errorf("remote-cert-eku の %s には対応していません", arg(1))
			}
		case "remote-cert-ku":
			c.RequireKU = true
			for _, v := range f[1:] {
				ku, err := strconv.ParseUint(v, 16, 32)
				if err != nil {
					return nil, fmt.Errorf("remote-cert-ku の %s を読めません", v)
				}
				c.KeyUsages = append(c.KeyUsages, uint(ku))
			}
		case "ns-cert-type":
			// サーバーの用途は、いつも確かめている (verifyServer)
			if arg(1) != "server" {
				return nil, fmt.Errorf("ns-cert-type %s には対応していません", arg(1))
			}
		case "verify-hash", "peer-fingerprint":
			if _, ok := blocks[f[0]]; ok {
				break
			}
			algo := arg(2)
			if f[0] == "verify-hash" && algo == "" {
				algo = "SHA1"
			}
			if err := c.addHashes(f[0], arg(1), algo); err != nil {
				return nil, err
			}
		case "crl-verify":
			if arg(2) == "dir" {
				return nil, errors.New("crl-verify の dir には対応していません。CRL を .ovpn の中に <crl-verify> で書いてください")
			}
			if _, ok := blocks["crl-verify"]; !ok {
				return nil, errors.New("crl-verify の CRL を .ovpn の中に <crl-verify> で書いてください")
			}
		case "tls-verify":
			return nil, errors.New("tls-verify (スクリプトで証明書を確かめる形) には対応していません")
		case "verify-x509-name":
			switch arg(2) {
			case "", "name":
			case "name-prefix":
				c.VerifyPrefix = true
			default:
				return nil, fmt.Errorf("verify-x509-name の %s には対応していません", arg(2))
			}
			c.VerifyName = arg(1)
		case "key-direction":
			c.TLSAuthDir, _ = strconv.Atoi(arg(1))
		case "tls-auth":
			if d := arg(2); d != "" {
				c.TLSAuthDir, _ = strconv.Atoi(d)
			}
			if _, ok := blocks["tls-auth"]; !ok {
				return nil, errors.New("tls-auth の鍵を .ovpn の中に <tls-auth> で書いてください")
			}
		case "tls-crypt":
			if _, ok := blocks["tls-crypt"]; !ok {
				return nil, errors.New("tls-crypt の鍵を .ovpn の中に <tls-crypt> で書いてください")
			}
		case "auth":
			c.Auth = strings.ToUpper(arg(1))
		case "cipher":
			c.Cipher = strings.ToUpper(arg(1))
		case "data-ciphers", "ncp-ciphers":
			c.DataCiphers = strings.Split(strings.ToUpper(arg(1)), ":")
		case "auth-user-pass":
			c.AuthUserPass = true
		case "comp-lzo":
			c.Comp = compLZO
		case "compress":
			c.Comp = compressMode(arg(1))
		case "fragment":
			c.Fragment, _ = strconv.Atoi(arg(1))
			if arg(2) == "mtu" { // IP と UDP の頭も含めた値
				c.Fragment -= 28
			}
		case "ca", "cert", "key":
			if _, ok := blocks[f[0]]; !ok {
				return nil, fmt.Errorf("%s を .ovpn の中に <%s> で書いてください", f[0], f[0])
			}
		}
	}
	if len(c.Remotes) == 0 {
		return nil, errors.New("remote の行がありません")
	}
	// 埋め込みの指紋は、OpenVPN と同じく SHA256 で読む
	for _, name := range []string{"verify-hash", "peer-fingerprint"} {
		if b, ok := blocks[name]; ok {
			if err := c.addHashes(name, b, "SHA256"); err != nil {
				return nil, err
			}
		}
	}
	if b, ok := blocks["crl-verify"]; ok {
		rest := []byte(b)
		for {
			var p *pem.Block
			if p, rest = pem.Decode(rest); p == nil {
				break
			}
			rl, err := x509.ParseRevocationList(p.Bytes)
			if err != nil {
				return nil, fmt.Errorf("<crl-verify> を読めません: %w", err)
			}
			c.CRLs = append(c.CRLs, rl)
		}
		if len(c.CRLs) == 0 {
			return nil, errors.New("<crl-verify> に CRL がありません")
		}
	}
	// peer-fingerprint だけでサーバーを決めるときは、CA を使わない
	if ca := blocks["ca"]; ca != "" {
		c.CA = x509.NewCertPool()
		if !c.CA.AppendCertsFromPEM([]byte(ca)) {
			return nil, errors.New("<ca> の証明書を読めません")
		}
	} else if c.HashDepth != 0 || len(c.Hashes) == 0 {
		return nil, errors.New("<ca> がありません")
	}
	if c.CA == nil && len(c.CRLs) > 0 {
		return nil, errors.New("<crl-verify> を使うには <ca> が要ります")
	}
	if cert, key := blocks["cert"], blocks["key"]; cert != "" || key != "" {
		kp, err := tls.X509KeyPair([]byte(cert), []byte(key))
		if err != nil {
			return nil, fmt.Errorf("<cert> と <key> を読めません: %w", err)
		}
		c.Cert = &kp
	}
	if c.Cert == nil && !c.AuthUserPass {
		return nil, errors.New("クライアント証明書 (<cert> と <key>) も auth-user-pass もありません")
	}
	var err error
	if b := blocks["tls-auth"]; b != "" {
		if c.TLSAuth, err = parseStaticKey(b); err != nil {
			return nil, fmt.Errorf("<tls-auth> を読めません: %w", err)
		}
		if _, ok := digests[c.Auth]; !ok {
			return nil, fmt.Errorf("auth %s には対応していません", c.Auth)
		}
	}
	if b := blocks["tls-crypt"]; b != "" {
		if c.TLSCrypt, err = parseStaticKey(b); err != nil {
			return nil, fmt.Errorf("<tls-crypt> を読めません: %w", err)
		}
	}
	if b := blocks["auth-user-pass"]; b != "" {
		up := strings.SplitN(b, "\n", 3)
		c.AuthUserPass = true
		c.User = strings.TrimSpace(up[0])
		if len(up) > 1 {
			c.Pass = strings.TrimSpace(up[1])
		}
	}
	return c, nil
}

// サーバーとクライアントの用途の OID
const (
	oidServerAuth = "1.3.6.1.5.5.7.3.1"
	oidClientAuth = "1.3.6.1.5.5.7.3.2"
)

// ekuOID は remote-cert-eku の値を OID にする。OpenVPN は OID か OpenSSL の長い名前で書く。読めなければ空
func ekuOID(v string) string {
	switch v {
	case "TLS Web Server Authentication":
		return oidServerAuth
	case "TLS Web Client Authentication":
		return oidClientAuth
	}
	var oid asn1.ObjectIdentifier
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return ""
		}
		oid = append(oid, n)
	}
	if len(oid) < 2 {
		return ""
	}
	return oid.String()
}

// addHashes は verify-hash か peer-fingerprint の指紋を足す。text は1行に1つで、AB:CD:... の形
func (c *Config) addHashes(name, text, algo string) error {
	depth, size := 0, sha256.Size
	if name == "verify-hash" {
		depth = 1
		switch algo {
		case "SHA1":
			size = sha1.Size
		case "SHA256":
		default:
			return fmt.Errorf("verify-hash の %s には対応していません", algo)
		}
	}
	if len(c.Hashes) > 0 && c.HashDepth != depth {
		return errors.New("verify-hash と peer-fingerprint は一緒に使えません")
	}
	c.HashDepth = depth
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l[0] == '#' || l[0] == ';' {
			continue
		}
		h, err := hex.DecodeString(strings.ReplaceAll(l, ":", ""))
		if err != nil || len(h) != size {
			return fmt.Errorf("%s の指紋 %s を読めません", name, l)
		}
		c.Hashes = append(c.Hashes, h)
	}
	if len(c.Hashes) == 0 {
		return fmt.Errorf("%s に指紋がありません", name)
	}
	return nil
}

func compressMode(alg string) compMode {
	switch alg {
	case "lzo":
		return compLZO
	case "stub-v2", "lz4-v2":
		return compV2
	case "migrate":
		return compNone
	}
	return compSwap // 引数なし、stub、lz4
}

// fields は1行を空白で区切る。"..." で囲んだところは1つにする
func fields(line string) []string {
	var out []string
	var b strings.Builder
	quoted, in := false, false
	for _, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
			in = true
		case !quoted && (r == ' ' || r == '\t'):
			if in {
				out = append(out, b.String())
				b.Reset()
				in = false
			}
		default:
			b.WriteRune(r)
			in = true
		}
	}
	if in {
		out = append(out, b.String())
	}
	return out
}

func parseStaticKey(text string) (*staticKey, error) {
	if p, _ := pem.Decode([]byte(text)); p != nil && p.Type != "OpenVPN Static key V1" {
		return nil, fmt.Errorf("%s は使えません", p.Type)
	}
	var hx strings.Builder
	in := false
	for _, l := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(l, "-----BEGIN"):
			in = true
		case strings.HasPrefix(l, "-----END"):
			in = false
		case in:
			hx.WriteString(strings.TrimSpace(l))
		}
	}
	b, err := hex.DecodeString(hx.String())
	if err != nil || len(b) != 256 {
		return nil, errors.New("鍵の形が違います")
	}
	var k staticKey
	for i := range k {
		copy(k[i].cipher[:], b[i*128:])
		copy(k[i].hmac[:], b[i*128+64:])
	}
	return &k, nil
}
