// Package sstp は SSTP ([MS-SSTP]) のクライアント。HTTPS の上に SSTP の枠を張り、その中で PPP を話す
package sstp

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/asutarisucu/DRAG/app/internal/vpn/ppp"
)

// Config は SSTP でつなぐ設定
type Config struct {
	Server     string         // host か host:port。port が無ければ 443
	User, Pass string         // PPP の認証 (MS-CHAPv2 など) のユーザー名とパスワード
	CA         *x509.CertPool // サーバーの証明書を確かめる CA。nil なら Windows が信頼する CA
	Logf       func(format string, args ...any)
}

// メッセージの種類 ([MS-SSTP] 2.2.2)
const (
	msgConnectRequest = 1
	msgConnectAck     = 2
	msgConnectNak     = 3
	msgConnected      = 4
	msgAbort          = 5
	msgDisconnect     = 6
	msgDisconnectAck  = 7
	msgEchoRequest    = 8
	msgEchoResponse   = 9
)

// Client は張った SSTP の VPN 1つ
type Client struct {
	cfg      Config
	conn     *tls.Conn
	r        *bufio.Reader
	wmu      sync.Mutex
	certHash []byte // サーバーの証明書の SHA-256
	ppp      *ppp.Conn
	ack      chan []byte // Call Connect Acknowledge の中身。Nak のときは nil
	nonce    []byte
	pushed   ppp.Result

	done chan struct{}
	once sync.Once
	err  error
}

func (c *Client) logf(format string, args ...any) {
	if c.cfg.Logf != nil {
		c.cfg.Logf(format, args...)
	}
}

// Dial は SSTP でつなぎ、PPP でアドレスをもらうまで待つ
func Dial(ctx context.Context, cfg Config) (*Client, error) {
	host, port := cfg.Server, "443"
	if h, p, err := net.SplitHostPort(cfg.Server); err == nil {
		host, port = h, p
	}
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}, Config: &tls.Config{ServerName: host, RootCAs: cfg.CA, MinVersion: tls.VersionTLS12}}
	nc, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, fmt.Errorf("VPN のサーバーと HTTPS でつなげません: %w", err)
	}
	conn := nc.(*tls.Conn)
	leaf := conn.ConnectionState().PeerCertificates[0]
	sum := sha256.Sum256(leaf.Raw)
	c := &Client{cfg: cfg, conn: conn, r: bufio.NewReader(conn), certHash: sum[:], ack: make(chan []byte, 1), done: make(chan struct{})}
	if err := c.handshake(ctx, host); err != nil {
		c.fail(err)
		conn.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) handshake(ctx context.Context, host string) error {
	if dl, ok := ctx.Deadline(); ok {
		c.conn.SetDeadline(dl)
	}
	// HTTPS の上に、SSTP の双方向の通り道を開く ([MS-SSTP] 4.1)
	guid := randBytes(16)
	req := "SSTP_DUPLEX_POST /sra_{BA195980-CD49-458b-9E23-C84EE0ADCD75}/ HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Content-Length: 18446744073709551615\r\n" +
		fmt.Sprintf("SSTPCORRELATIONID: {%X-%X-%X-%X-%X}\r\n\r\n", guid[0:4], guid[4:6], guid[6:8], guid[8:10], guid[10:16])
	if _, err := io.WriteString(c.conn, req); err != nil {
		return err
	}
	// Content-Length が 18446744073709551615 なので net/http では読めない。状態の行とヘッダーを自分で読み飛ばす
	status, err := c.r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("VPN のサーバーが SSTP の HTTP に答えません: %w", err)
	}
	if f := strings.Fields(status); len(f) < 2 || f[1] != "200" {
		return fmt.Errorf("VPN のサーバーが SSTP を受け付けません (%s)", strings.TrimSpace(status))
	}
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			return fmt.Errorf("VPN のサーバーが SSTP の HTTP に答えません: %w", err)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	c.conn.SetDeadline(time.Time{})

	c.ppp = ppp.New(ppp.Config{User: c.cfg.User, Pass: c.cfg.Pass, OnAuth: c.connected, Logf: c.cfg.Logf}, c.writeData)
	go c.readLoop()
	// PPP が終わったら SSTP も終える
	go func() {
		select {
		case <-c.ppp.Done():
			c.fail(c.ppp.Err())
		case <-c.done:
		}
	}()
	// Encapsulated Protocol ID の属性 (PPP) を1つだけ付ける
	if err := c.writeControl(msgConnectRequest, attr(1, []byte{0, 1})); err != nil {
		return err
	}
	select {
	case ack := <-c.ack:
		if ack == nil {
			return errors.New("VPN のサーバーが SSTP の接続を断りました")
		}
		// Crypto Binding Request の属性: 予約 3 バイト、ハッシュの種類、nonce 32 バイト
		if len(ack) < 4+4+32 || ack[4+3]&0x02 == 0 {
			return errors.New("VPN のサーバーが SHA-256 の Crypto Binding に対応していません")
		}
		c.nonce = append([]byte{}, ack[8:40]...)
	case <-c.done:
		return c.err
	case <-ctx.Done():
		return errors.New("VPN のサーバーから SSTP の返事がありません")
	}
	// サーバーが SSTP を切ったら、PPP を待つのもやめる
	pctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-c.done:
			cancel()
		case <-pctx.Done():
		}
	}()
	res, err := c.ppp.Start(pctx)
	if err != nil {
		select {
		case <-c.done:
			return c.err
		default:
		}
		return err
	}
	c.pushed = res
	return nil
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func attr(id byte, value []byte) []byte {
	b := []byte{0, id, 0, 0}
	binary.BigEndian.PutUint16(b[2:4], uint16(4+len(value)))
	return append(b, value...)
}

func (c *Client) writePacket(control bool, body []byte) error {
	b := []byte{0x10, 0, 0, 0}
	if control {
		b[1] = 1
	}
	binary.BigEndian.PutUint16(b[2:4], uint16(4+len(body)))
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err := c.conn.Write(append(b, body...))
	return err
}

func (c *Client) writeControl(typ uint16, attrs ...[]byte) error {
	body := binary.BigEndian.AppendUint16(nil, typ)
	body = binary.BigEndian.AppendUint16(body, uint16(len(attrs)))
	for _, a := range attrs {
		body = append(body, a...)
	}
	return c.writePacket(true, body)
}

func (c *Client) writeData(frame []byte) error { return c.writePacket(false, frame) }

// connected は、PPP の認証が済んだら Call Connected で Crypto Binding を送る ([MS-SSTP] 3.2.5.2)
func (c *Client) connected(hlak []byte) error {
	value := []byte{0, 0, 0, 2} // 予約 3 バイトと、SHA-256
	value = append(value, c.nonce...)
	value = append(value, c.certHash...)
	value = append(value, make([]byte, 32)...) // Compound MAC は計算のあいだ 0 にしておく
	a := attr(3, value)
	msg := []byte{0x10, 1, 0, 0}
	msg = append(msg, 0, msgConnected, 0, 1)
	msg = append(msg, a...)
	binary.BigEndian.PutUint16(msg[2:4], uint16(len(msg)))
	m := hmac.New(sha256.New, cmk(hlak))
	m.Write(msg)
	copy(msg[len(msg)-32:], m.Sum(nil))
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err := c.conn.Write(msg)
	c.logf("SSTP の Crypto Binding を送りました")
	return err
}

// cmk は Compound MAC の鍵。IKEv2 の prf+ と似ているが、長さを 2 バイト (little endian) で足す
func cmk(hlak []byte) []byte {
	seed := []byte("SSTP inner method derived CMK")
	m := hmac.New(sha256.New, hlak)
	m.Write(seed)
	m.Write([]byte{32, 0, 1})
	return m.Sum(nil)
}

func (c *Client) readLoop() {
	for {
		var h [4]byte
		if _, err := io.ReadFull(c.r, h[:]); err != nil {
			c.fail(fmt.Errorf("VPN のサーバーとの通信が切れました: %w", err))
			return
		}
		n := int(binary.BigEndian.Uint16(h[2:4]) & 0x0fff)
		if n < 4 {
			c.fail(errors.New("SSTP のパケットを読めません"))
			return
		}
		body := make([]byte, n-4)
		if _, err := io.ReadFull(c.r, body); err != nil {
			c.fail(fmt.Errorf("VPN のサーバーとの通信が切れました: %w", err))
			return
		}
		if h[1]&1 == 0 {
			c.ppp.Input(body)
			continue
		}
		if len(body) < 4 {
			continue
		}
		switch typ := binary.BigEndian.Uint16(body[:2]); typ {
		case msgConnectAck:
			select {
			case c.ack <- body[4:]:
			default:
			}
		case msgConnectNak:
			select {
			case c.ack <- nil:
			default:
			}
		case msgEchoRequest:
			c.writeControl(msgEchoResponse)
		case msgDisconnect:
			c.writeControl(msgDisconnectAck)
			c.fail(errors.New("VPN のサーバーが SSTP を終えました"))
			return
		case msgAbort:
			c.fail(errors.New("VPN のサーバーが SSTP を打ち切りました"))
			return
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
func (c *Client) WriteIP(p []byte) error { return c.ppp.WriteIP(p) }

// ReadIP は VPN から届いた IP のパケットを1つ返す
func (c *Client) ReadIP() ([]byte, error) {
	select {
	case <-c.done:
		return nil, c.err
	default:
	}
	return c.ppp.ReadIP()
}

// Pushed は PPP でもらったアドレスと DNS
func (c *Client) Pushed() ppp.Result { return c.pushed }

func (c *Client) Done() <-chan struct{} { return c.done }

func (c *Client) Err() error {
	<-c.done
	return c.err
}

// Close は PPP と SSTP を終える
func (c *Client) Close() error {
	select {
	case <-c.done:
		return nil
	default:
	}
	c.ppp.Close()
	c.writeControl(msgDisconnect)
	c.fail(net.ErrClosed)
	return nil
}
