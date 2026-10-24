// lanserve は PC アプリの代わりに LAN に HTTPS で待ち受け、スマホのブラウザに検証用サーバーのシェルを出す。
// 確かめること
//   - PC アプリが作る自己署名の証明書で、スマホのブラウザが HTTPS と WebSocket をつなげるか
//   - Windows のファイアウォールの確認がどう出るか
//
// 端末の登録の代わりに、起動のたびに作る合言葉を URL に付ける。合言葉を持たない接続は断る。
// 決めた時間がたつと終わる
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"
)

var (
	listen  = flag.String("listen", ":8443", "待ち受けるアドレス")
	sshAddr = flag.String("ssh", "127.0.0.1:2222", "SSH の接続先")
	user    = flag.String("user", "asuta", "ユーザー名")
	pass    = flag.String("pass", "drag", "パスワード")
	minutes = flag.Int("minutes", 30, "この分数がたったら終わる")
)

//go:embed index.html
var page []byte

var token = rand.Text()

// lanIPs は、ループバックでない IPv4 アドレスを返す
func lanIPs() []net.IP {
	var ips []net.IP
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() {
			ips = append(ips, n.IP.To4())
		}
	}
	return ips
}

// makeCert は自己署名の証明書を作る。
// iOS は SAN と serverAuth が無いもの、有効期間が 825 日を超えるものを受け付けないので、それに合わせる
func makeCert(ips []net.IP) (tls.Certificate, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "DRAG", Organization: []string{"DRAG"}},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(397 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  append([]net.IP{net.IPv4(127, 0, 0, 1)}, ips...),
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	sum := sha256.Sum256(der)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, fmt.Sprintf("%X", sum), nil
}

func hasToken(v string) bool {
	return subtle.ConstantTimeCompare([]byte(v), []byte(token)) == 1
}

func cookieToken(r *http.Request) string {
	if c, err := r.Cookie("drag"); err == nil {
		return c.Value
	}
	return ""
}

func main() {
	flag.Parse()
	sc, err := ssh.Dial("tcp", *sshAddr, &ssh.ClientConfig{
		User:            *user,
		Auth:            []ssh.AuthMethod{ssh.Password(*pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}
	ips := lanIPs()
	cert, fp, err := makeCert(ips)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		// QR コードで開く URL の代わり。合言葉を cookie に移して、URL から消す
		if t := r.URL.Query().Get("t"); t != "" {
			if !hasToken(t) {
				http.Error(w, "合言葉が違います", http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "drag", Value: token, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: 3600})
			http.Redirect(w, r, "/?registered=1", http.StatusSeeOther)
			return
		}
		if !hasToken(cookieToken(r)) {
			http.Error(w, "この端末は登録されていません", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	})
	// ページが調べた結果を受け取って、PC の側に出す
	mux.HandleFunc("POST /report", func(w http.ResponseWriter, r *http.Request) {
		if !hasToken(cookieToken(r)) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		b, _ := io.ReadAll(io.LimitReader(r.Body, 8192))
		log.Printf("report %s %s", r.RemoteAddr, b)
	})
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		u, err := url.Parse(origin)
		if origin == "" || err != nil || !strings.EqualFold(u.Host, r.Host) {
			log.Printf("ws %s 接続元が違う %q", r.RemoteAddr, origin)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if !hasToken(cookieToken(r)) {
			log.Printf("ws %s cookie がない", r.RemoteAddr)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
		rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
		if cols <= 0 || rows <= 0 {
			cols, rows = 120, 30
		}
		ws, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer ws.CloseNow()
		log.Printf("ws %s つないだ %dx%d %s", r.RemoteAddr, cols, rows, r.UserAgent())
		relay(ws, sc, cols, rows)
		log.Printf("ws %s 切れた", r.RemoteAddr)
	})

	srv := &http.Server{
		Addr:      *listen,
		Handler:   mux,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}
	_, port, _ := net.SplitHostPort(*listen)
	fmt.Printf("証明書の SHA-256  %s\n", fp)
	for _, ip := range ips {
		fmt.Printf("https://%s:%s/?t=%s\n", ip, port, token)
	}
	fmt.Printf("https://127.0.0.1:%s/?t=%s\n", port, token)
	time.AfterFunc(time.Duration(*minutes)*time.Minute, func() {
		log.Print("時間になったので終わる")
		os.Exit(0)
	})
	log.Fatal(srv.ListenAndServeTLS("", ""))
}

// relay は WebSocket の1本を、SSH の PTY 付きのシェル1つにつなぐ。
// バイナリのメッセージはキー入力、テキストは {"cols":..,"rows":..} の大きさの変更
func relay(ws *websocket.Conn, sc *ssh.Client, cols, rows int) {
	s, err := sc.NewSession()
	if err != nil {
		ws.Close(websocket.StatusInternalError, err.Error())
		return
	}
	defer s.Close()
	if err := s.RequestPty("xterm-256color", rows, cols, nil); err != nil {
		ws.Close(websocket.StatusInternalError, err.Error())
		return
	}
	in, _ := s.StdinPipe()
	out, _ := s.StdoutPipe()
	s.Stderr = s.Stdout
	if err := s.Shell(); err != nil {
		ws.Close(websocket.StatusInternalError, err.Error())
		return
	}
	ws.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer s.Close()
		for {
			typ, p, err := ws.Read(ctx)
			if err != nil {
				return
			}
			if typ == websocket.MessageBinary {
				in.Write(p)
				continue
			}
			var sz struct{ Cols, Rows int }
			if json.Unmarshal(p, &sz) == nil && sz.Cols > 0 && sz.Rows > 0 {
				s.WindowChange(sz.Rows, sz.Cols)
			}
		}
	}()

	b := make([]byte, 32*1024)
	for {
		n, err := out.Read(b)
		if n > 0 {
			if werr := ws.Write(ctx, websocket.MessageBinary, b[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			ws.Close(websocket.StatusNormalClosure, "shell exited")
			return
		}
	}
}
