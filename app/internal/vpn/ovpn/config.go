// Package ovpn は OpenVPN のクライアント。TLS モードで、サーバーとのやり取りと
// データチャネルの暗号を Go で行い、IP のパケットを読み書きする口を出す。
// 仕様は OpenVPN/openvpn-rfc の openvpn-wire-protocol.xml。仕様に無い部分 (圧縮の印や ping の中身) は
// OpenVPN v2.6.23 の挙動に合わせている。コードは写していない
package ovpn

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
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
	Cert         *tls.Certificate // クライアント証明書。ユーザー名とパスワードだけで入るときは nil
	ServerEKU    bool             // remote-cert-tls server
	VerifyName   string           // verify-x509-name の値
	VerifyPrefix bool             // verify-x509-name の name-prefix
	TLSAuth      *staticKey
	TLSAuthDir   int // key-direction。-1 なら向きを分けない
	TLSCrypt     *staticKey
	Auth         string // tls-auth と CBC の HMAC のハッシュ。既定は SHA1
	Cipher       string // サーバーが cipher を送ってこないときに使う
	DataCiphers  []string
	AuthUserPass bool
	User, Pass   string // <auth-user-pass> に書いてあったとき
	Comp         compMode
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
			c.ServerEKU = arg(1) == "server"
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
		case "ca", "cert", "key":
			if _, ok := blocks[f[0]]; !ok {
				return nil, fmt.Errorf("%s を .ovpn の中に <%s> で書いてください", f[0], f[0])
			}
		}
	}
	if len(c.Remotes) == 0 {
		return nil, errors.New("remote の行がありません")
	}
	ca := blocks["ca"]
	if ca == "" {
		return nil, errors.New("<ca> がありません")
	}
	c.CA = x509.NewCertPool()
	if !c.CA.AppendCertsFromPEM([]byte(ca)) {
		return nil, errors.New("<ca> の証明書を読めません")
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
