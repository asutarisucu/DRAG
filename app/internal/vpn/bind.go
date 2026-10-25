package vpn

import (
	"errors"
	"net"
	"net/netip"
	"sync"

	"golang.zx2c4.com/wireguard/conn"
)

// dialBind は wireguard-go の conn.Bind。相手ごとに、その相手へ net.DialUDP したソケットで送り受けする。
// 待ち受けのソケットは開かない。VPN の相手は設定の Endpoint だけなので、これで足りる
type dialBind struct {
	mu     sync.Mutex
	conns  map[netip.AddrPort]*net.UDPConn
	recv   chan udpPacket
	closed chan struct{}
}

type udpPacket struct {
	b    []byte
	from netip.AddrPort
}

func newDialBind() *dialBind {
	return &dialBind{}
}

func (b *dialBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed != nil {
		return nil, 0, conn.ErrBindAlreadyOpen
	}
	b.conns = map[netip.AddrPort]*net.UDPConn{}
	b.recv = make(chan udpPacket, 256)
	b.closed = make(chan struct{})
	recv, closed := b.recv, b.closed
	fn := func(packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		select {
		case p := <-recv:
			sizes[0] = copy(packets[0], p.b)
			eps[0] = udpEndpoint(p.from)
			return 1, nil
		case <-closed:
			return 0, net.ErrClosed
		}
	}
	return []conn.ReceiveFunc{fn}, port, nil
}

func (b *dialBind) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed == nil {
		return nil
	}
	close(b.closed)
	for _, c := range b.conns {
		c.Close()
	}
	b.conns, b.recv, b.closed = nil, nil, nil
	return nil
}

func (b *dialBind) SetMark(uint32) error { return nil }

func (b *dialBind) BatchSize() int { return 1 }

func (b *dialBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	ap, err := netip.ParseAddrPort(s)
	if err != nil {
		return nil, err
	}
	return udpEndpoint(ap), nil
}

func (b *dialBind) Send(bufs [][]byte, ep conn.Endpoint) error {
	e, ok := ep.(udpEndpoint)
	if !ok {
		return conn.ErrWrongEndpointType
	}
	c, err := b.conn(netip.AddrPort(e))
	if err != nil {
		return err
	}
	for _, buf := range bufs {
		if _, err := c.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

// conn は相手へのソケット。初めて送るときにつなぎ、受けたものを recv へ流す
func (b *dialBind) conn(to netip.AddrPort) (*net.UDPConn, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed == nil {
		return nil, net.ErrClosed
	}
	if c := b.conns[to]; c != nil {
		return c, nil
	}
	c, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(to))
	if err != nil {
		return nil, err
	}
	b.conns[to] = c
	recv, closed := b.recv, b.closed
	go func() {
		buf := make([]byte, 65535)
		for {
			n, err := c.Read(buf)
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if err != nil {
				// 相手のポートが閉じていると、Windows では ICMP を受けて読み出しが失敗する。そのまま読み続ける
				continue
			}
			select {
			case recv <- udpPacket{b: append([]byte(nil), buf[:n]...), from: to}:
			case <-closed:
				return
			}
		}
	}()
	return c, nil
}

// udpEndpoint は相手のアドレス。送る元のアドレスは OS に任せる
type udpEndpoint netip.AddrPort

func (e udpEndpoint) ClearSrc()           {}
func (e udpEndpoint) SrcToString() string { return "" }
func (e udpEndpoint) DstToString() string { return netip.AddrPort(e).String() }
func (e udpEndpoint) DstToBytes() []byte {
	b, _ := netip.AddrPort(e).MarshalBinary()
	return b
}
func (e udpEndpoint) DstIP() netip.Addr { return netip.AddrPort(e).Addr() }
func (e udpEndpoint) SrcIP() netip.Addr { return netip.Addr{} }
