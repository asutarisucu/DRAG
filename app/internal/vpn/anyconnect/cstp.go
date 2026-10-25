package anyconnect

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config は AnyConnect 互換でつなぐ設定
type Config struct {
	Server     string         // host、host:port、https://host/path のどれか
	Group      string         // ログインで選ぶグループ。空ならサーバーの既定
	User, Pass string         // ログインのユーザー名とパスワード
	CA         *x509.CertPool // サーバーの証明書を確かめる CA。nil なら Windows が信頼する CA
	MTU        int            // こちらから送る IP のパケットの大きさの上限
	Logf       func(format string, args ...any)
}

// CSTP のパケットの種類 (openconnect v9.12 openconnect-internal.h の AC_PKT_*)
const (
	pktData       = 0
	pktDPDOut     = 3
	pktDPDResp    = 4
	pktDisconnect = 5
	pktKeepalive  = 7
	pktCompressed = 8
	pktTerminate  = 9
)

// Pushed は、つないだときにサーバーが決めたもの
type Pushed struct {
	Addr   netip.Addr
	DNS    []netip.Addr
	Routes []netip.Prefix // 空なら全部を VPN に通す
	MTU    int
}

// Client は張った AnyConnect 互換の VPN 1つ
type Client struct {
	cfg    Config
	conn   *tls.Conn
	r      *bufio.Reader
	wmu    sync.Mutex
	pushed Pushed
	in     chan []byte

	dpd, keepalive time.Duration
	mu             sync.Mutex
	lastRx, lastTx time.Time

	done chan struct{}
	once sync.Once
	err  error
}

func (c *Client) logf(format string, args ...any) {
	if c.cfg.Logf != nil {
		c.cfg.Logf(format, args...)
	}
}

// serverURL は、入れたサーバーを https の URL にする
func serverURL(s string) (*url.URL, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return nil, fmt.Errorf("VPN のサーバーを読めません: %s", s)
	}
	if u.Port() == "" {
		u.Host = net.JoinHostPort(u.Hostname(), "443")
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u, nil
}

// Dial は、ログインしてから CSTP のトンネルを開き、アドレスをもらうまで待つ
func Dial(ctx context.Context, cfg Config) (*Client, error) {
	base, err := serverURL(cfg.Server)
	if err != nil {
		return nil, err
	}
	tc := &tls.Config{ServerName: base.Hostname(), RootCAs: cfg.CA, MinVersion: tls.VersionTLS12, Renegotiation: tls.RenegotiateFreelyAsClient}
	cookie, err := login(ctx, cfg, base, tc)
	if err != nil {
		return nil, err
	}
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}, Config: tc}
	nc, err := d.DialContext(ctx, "tcp", base.Host)
	if err != nil {
		return nil, fmt.Errorf("VPN のサーバーと HTTPS でつなげません: %w", err)
	}
	c := &Client{cfg: cfg, conn: nc.(*tls.Conn), in: make(chan []byte, 512), done: make(chan struct{})}
	c.r = bufio.NewReader(c.conn)
	if err := c.connect(ctx, base, cookie); err != nil {
		c.conn.Close()
		return nil, err
	}
	now := time.Now()
	c.lastRx, c.lastTx = now, now
	go c.readLoop()
	go c.timers()
	return c, nil
}

// connect は CONNECT /CSCOSSLC/tunnel を送り、返ってきた X-CSTP-* を読む
func (c *Client) connect(ctx context.Context, base *url.URL, cookie string) error {
	if dl, ok := ctx.Deadline(); ok {
		c.conn.SetDeadline(dl)
	}
	defer c.conn.SetDeadline(time.Time{})
	host := base.Host
	if base.Port() == "443" {
		host = base.Hostname()
	}
	name, err := os.Hostname()
	if err != nil || name == "" {
		name = "drag"
	}
	req := "CONNECT /CSCOSSLC/tunnel HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"User-Agent: " + userAgent + "\r\n" +
		"Cookie: webvpn=" + cookie + "\r\n" +
		"X-CSTP-Version: 1\r\n" +
		"X-CSTP-Hostname: " + name + "\r\n" +
		"X-CSTP-Address-Type: IPv4\r\n"
	if c.cfg.MTU > 0 {
		req += "X-CSTP-MTU: " + strconv.Itoa(c.cfg.MTU) + "\r\n"
	}
	req += "\r\n"
	if _, err := io.WriteString(c.conn, req); err != nil {
		return fmt.Errorf("VPN のサーバーとの通信が切れました: %w", err)
	}
	status, err := c.r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("VPN のサーバーがトンネルを開きません: %w", err)
	}
	if f := strings.Fields(status); len(f) < 2 || f[1] != "200" {
		return fmt.Errorf("VPN のサーバーがトンネルを開きません (%s)", strings.TrimSpace(status))
	}
	p := &c.pushed
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("VPN のサーバーとの通信が切れました: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "X-CSTP-Address":
			if a, err := netip.ParseAddr(v); err == nil && a.Is4() {
				p.Addr = a
			}
		case "X-CSTP-DNS":
			if a, err := netip.ParseAddr(v); err == nil && a.Is4() {
				p.DNS = append(p.DNS, a)
			}
		case "X-CSTP-Split-Include":
			if r, ok := parseRoute(v); ok {
				p.Routes = append(p.Routes, r)
			}
		case "X-CSTP-MTU":
			p.MTU, _ = strconv.Atoi(v)
		case "X-CSTP-DPD":
			n, _ := strconv.Atoi(v)
			c.dpd = time.Duration(n) * time.Second
		case "X-CSTP-Keepalive":
			n, _ := strconv.Atoi(v)
			c.keepalive = time.Duration(n) * time.Second
		case "X-CSTP-Content-Encoding":
			// 圧縮は求めていないので、送られても戻せない
			return fmt.Errorf("VPN のサーバーが圧縮を使います (%s)。DRAG はまだ対応していません", v)
		}
	}
	if !p.Addr.IsValid() {
		return errors.New("VPN のサーバーが IPv4 のアドレスをくれません")
	}
	c.logf("AnyConnect 互換のトンネルを開きました。この PC は %s", p.Addr)
	return nil
}

// parseRoute は 192.168.50.0/255.255.255.0 か 192.168.50.0/24 を読む
func parseRoute(s string) (netip.Prefix, bool) {
	a, m, ok := strings.Cut(s, "/")
	ip, err := netip.ParseAddr(a)
	if !ok || err != nil || !ip.Is4() {
		return netip.Prefix{}, false
	}
	if n, err := strconv.Atoi(m); err == nil {
		r, err := ip.Prefix(n)
		return r, err == nil
	}
	mask, err := netip.ParseAddr(m)
	if err != nil || !mask.Is4() {
		return netip.Prefix{}, false
	}
	b := mask.As4()
	ones, _ := net.IPMask(b[:]).Size()
	r, err := ip.Prefix(ones)
	return r, err == nil
}

func (c *Client) write(typ byte, payload []byte) error {
	b := make([]byte, 8, 8+len(payload))
	copy(b, "STF\x01")
	binary.BigEndian.PutUint16(b[4:6], uint16(len(payload)))
	b[6] = typ
	b = append(b, payload...)
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if _, err := c.conn.Write(b); err != nil {
		return err
	}
	c.mu.Lock()
	c.lastTx = time.Now()
	c.mu.Unlock()
	return nil
}

func (c *Client) readLoop() {
	for {
		var h [8]byte
		if _, err := io.ReadFull(c.r, h[:]); err != nil {
			c.fail(fmt.Errorf("VPN のサーバーとの通信が切れました: %w", err))
			return
		}
		if string(h[:4]) != "STF\x01" {
			c.fail(errors.New("CSTP のパケットを読めません"))
			return
		}
		body := make([]byte, binary.BigEndian.Uint16(h[4:6]))
		if _, err := io.ReadFull(c.r, body); err != nil {
			c.fail(fmt.Errorf("VPN のサーバーとの通信が切れました: %w", err))
			return
		}
		c.mu.Lock()
		c.lastRx = time.Now()
		c.mu.Unlock()
		switch h[6] {
		case pktData:
			select {
			case c.in <- body:
			case <-c.done:
				return
			default:
			}
		case pktDPDOut:
			c.write(pktDPDResp, nil)
		case pktDisconnect, pktTerminate:
			c.fail(errors.New("VPN のサーバーが接続を終えました"))
			return
		case pktCompressed:
			c.fail(errors.New("VPN のサーバーが圧縮したパケットを送りました。DRAG はまだ対応していません"))
			return
		}
	}
}

// timers は、サーバーが決めた間隔で Keepalive と DPD を送り、返事が途絶えたら切る
func (c *Client) timers() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	var lastDPD time.Time
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
		}
		now := time.Now()
		c.mu.Lock()
		rx, tx := c.lastRx, c.lastTx
		c.mu.Unlock()
		if c.dpd > 0 && now.Sub(rx) >= 3*c.dpd {
			c.fail(errors.New("VPN のサーバーから返事がありません"))
			return
		}
		if c.dpd > 0 && now.Sub(rx) >= c.dpd && now.Sub(lastDPD) >= c.dpd {
			lastDPD = now
			c.write(pktDPDOut, nil)
		} else if c.keepalive > 0 && now.Sub(tx) >= c.keepalive {
			c.write(pktKeepalive, nil)
		}
	}
}

func (c *Client) fail(err error) {
	c.once.Do(func() {
		c.err = err
		close(c.done)
		c.conn.Close()
	})
}

// WriteIP は IP のパケットを VPN に送る
func (c *Client) WriteIP(p []byte) error {
	select {
	case <-c.done:
		return c.err
	default:
	}
	return c.write(pktData, p)
}

// ReadIP は VPN から届いた IP のパケットを1つ返す
func (c *Client) ReadIP() ([]byte, error) {
	select {
	case p := <-c.in:
		return p, nil
	case <-c.done:
		return nil, c.err
	}
}

// Pushed は、つないだときにサーバーが決めたアドレス、DNS、通す範囲
func (c *Client) Pushed() Pushed { return c.pushed }

func (c *Client) Done() <-chan struct{} { return c.done }

func (c *Client) Err() error {
	<-c.done
	return c.err
}

// Close は、サーバーに切ると知らせてから終える
func (c *Client) Close() error {
	select {
	case <-c.done:
		return nil
	default:
	}
	// 先頭の 0xb0 は openconnect の cstp_bye と同じ
	c.write(pktDisconnect, append([]byte{0xb0}, "Client disconnect"...))
	c.fail(net.ErrClosed)
	return nil
}
