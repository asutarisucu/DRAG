// Package l2tp は L2TP/IPsec のクライアント。
// IKEv1 の Main Mode (事前共有キー) と Quick Mode で ESP の SA を作り (RFC 2409)、
// UDP で包んだトランスポートモードの ESP (RFC 3947、RFC 3948) の中で L2TP (RFC 2661) を開き、その中で PPP を話す。
// 待ち受けのソケットは開かず、サーバーの NAT-T のポートへつないだ UDP のソケットだけを使う
package l2tp

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/asutarisucu/DRAG/app/internal/vpn/ppp"
)

// Config は L2TP/IPsec でつなぐ設定
type Config struct {
	Server     string // host か host:port。port は NAT-T の番号で、無ければ 4500
	PSK        string // IPsec の事前共有キー
	User, Pass string // PPP の認証のユーザー名とパスワード
	Logf       func(format string, args ...any)
}

// Client は張った L2TP/IPsec の VPN 1つ
type Client struct {
	cfg        Config
	conn       *net.UDPConn
	local      [4]byte
	remote     [4]byte
	remotePort uint16
	lifetime   time.Duration // ESP の SA に出す寿命
	noRekey    bool          // 試験で、サーバーが始める Quick Mode を待つとき

	ispi, rspi [8]byte

	mu       sync.Mutex
	ike      *isakmpCrypto
	waiting  *waiter
	resend   map[uint32][]byte   // 自分が送った Quick Mode の最後のメッセージ
	qm       map[uint32]*qmState // サーバーが始めた Quick Mode
	inbound  map[uint32]*espSA
	outbound []*espSA          // 送る SA。最後がいちばん新しい
	later    map[uint32]*espSA // サーバーが始めた Quick Mode で、終わってから使う送る SA (受ける SPI ごと)
	pair     map[uint32]uint32 // 送る SPI に対の受ける SPI
	rekeying bool
	lastSent time.Time

	tunnel, peerTunnel   uint16
	session, peerSession uint16
	ns, nr               uint16
	unacked              []ctlSent
	ctlIn                chan ctlMsg
	seqData              bool
	dataNs               uint16

	ppp    *ppp.Conn
	pushed ppp.Result

	done chan struct{}
	once sync.Once
	err  error
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func (c *Client) logf(format string, args ...any) {
	if c.cfg.Logf != nil {
		c.cfg.Logf(format, args...)
	}
}

// Dial は IPsec の SA を作り、L2TP と PPP を開いて、アドレスをもらうまで待つ
func Dial(ctx context.Context, cfg Config) (*Client, error) {
	return dial(ctx, cfg, time.Hour, false)
}

func dial(ctx context.Context, cfg Config, lifetime time.Duration, noRekey bool) (*Client, error) {
	host, port := cfg.Server, "4500"
	if h, p, err := net.SplitHostPort(cfg.Server); err == nil {
		host, port = h, p
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New(host + " の IPv4 のアドレスが見つかりません")
	}
	ua, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(ips[0].String(), port))
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp4", nil, ua)
	if err != nil {
		return nil, err
	}
	c := &Client{
		cfg: cfg, conn: conn, remotePort: uint16(ua.Port), lifetime: lifetime, noRekey: noRekey,
		resend: map[uint32][]byte{}, qm: map[uint32]*qmState{}, inbound: map[uint32]*espSA{}, later: map[uint32]*espSA{}, pair: map[uint32]uint32{},
		ctlIn: make(chan ctlMsg, 8), done: make(chan struct{}), lastSent: time.Now(),
	}
	copy(c.local[:], conn.LocalAddr().(*net.UDPAddr).IP.To4())
	copy(c.remote[:], ua.IP.To4())
	go c.recvLoop()
	if err := c.open(ctx); err != nil {
		c.fail(err)
		return nil, err
	}
	return c, nil
}

func (c *Client) open(ctx context.Context) error {
	if err := c.mainMode(ctx); err != nil {
		return err
	}
	if err := c.quickMode(ctx); err != nil {
		return err
	}
	go c.timerLoop()
	c.ppp = ppp.New(ppp.Config{User: c.cfg.User, Pass: c.cfg.Pass, Logf: c.cfg.Logf}, c.sendFrame)
	go func() {
		select {
		case <-c.ppp.Done():
			c.fail(c.ppp.Err())
		case <-c.done:
		}
	}()
	if err := c.startL2TP(ctx); err != nil {
		return err
	}
	c.logf("L2TP のセッションを開きました")
	// サーバーが切ったら、PPP を待つのもやめる
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
		case n >= 4 && binary.BigEndian.Uint32(b) == 0: // 頭の 0 が 4 バイトなら IKE
			c.handleIKE(b[4:])
		case n >= 8:
			c.handleESP(b)
		}
	}
}

func (c *Client) handleESP(b []byte) {
	c.mu.Lock()
	sa := c.inbound[binary.BigEndian.Uint32(b)]
	c.mu.Unlock()
	if sa == nil {
		return
	}
	next, data, err := sa.open(b)
	if err != nil || next != protoUDP || len(data) < 8 {
		return
	}
	c.handleL2TP(data[8:])
}

// timerLoop は、L2TP の送り直し、NAT と L2TP の keepalive、ESP の SA の作り直しをする
func (c *Client) timerLoop() {
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case now := <-t.C:
			c.retransmit(now)
			c.mu.Lock()
			idle := now.Sub(c.lastSent)
			ready := c.peerTunnel != 0 && len(c.unacked) == 0
			var newest *espSA
			if n := len(c.outbound); n > 0 {
				newest = c.outbound[n-1]
			}
			rekey := !c.noRekey && !c.rekeying && newest != nil && newest.expires.Sub(now) < newest.life/10
			c.rekeying = c.rekeying || rekey
			c.mu.Unlock()
			// SoftEther は 150 秒 ESP が来ないと切る。L2TP の HELLO で ESP を流し、返事でサーバーが生きているかも見る
			if idle > 30*time.Second && ready {
				c.sendControl(0, avp(avpMessageType, u16(msgHello)))
			} else if idle > 20*time.Second {
				c.write([]byte{0xff})
			}
			if rekey {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					if err := c.quickMode(ctx); err != nil {
						c.logf("ESP の SA を作り直せません: %v", err)
					}
					c.mu.Lock()
					c.rekeying = false
					c.mu.Unlock()
				}()
			}
			c.dropExpired(now)
		}
	}
}

// dropExpired は寿命の切れた SA を外す。新しい SA があれば古い送る SA は使わない
func (c *Client) dropExpired(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for spi, sa := range c.inbound {
		if now.After(sa.expires.Add(time.Minute)) {
			delete(c.inbound, spi)
		}
	}
	for len(c.outbound) > 1 && now.After(c.outbound[0].expires) {
		delete(c.pair, c.outbound[0].spi)
		c.outbound = c.outbound[1:]
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

// Close は PPP と L2TP を終え、IPsec の SA を消すようサーバーに伝える
func (c *Client) Close() error {
	select {
	case <-c.done:
		return nil
	default:
	}
	c.ppp.Close()
	c.sendControl(0,
		avp(avpMessageType, u16(msgStopCCN)),
		avp(avpTunnelID, u16(c.tunnel)),
		avp(avpResultCode, u16(1)),
	)
	c.mu.Lock()
	k := c.ike
	var spis [][]byte
	for spi := range c.inbound {
		spis = append(spis, be32(spi))
	}
	c.mu.Unlock()
	if k != nil {
		if len(spis) > 0 {
			c.sendInfo(k, deletePayload(protoESP, spis...))
		}
		c.sendInfo(k, deletePayload(protoISAKMP, append(c.ispi[:], c.rspi[:]...)))
	}
	c.fail(net.ErrClosed)
	return nil
}
