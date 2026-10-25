package ovpn

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

// transport は OpenVPN のパケットを運ぶ UDP か TCP の接続
type transport interface {
	send(b []byte) error
	recv() ([]byte, error)
	close() error
}

func dialTransport(ctx context.Context, r Remote) (transport, error) {
	addrs, err := net.DefaultResolver.LookupHost(ctx, r.Host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, errors.New(r.Host + " のアドレスが見つかりません")
	}
	addr := net.JoinHostPort(addrs[0], r.Port)
	if r.TCP {
		d := net.Dialer{Timeout: 10 * time.Second}
		c, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, err
		}
		return &tcpTransport{c: c, r: bufio.NewReader(c)}, nil
	}
	// 相手へつないだ UDP のソケットだけを使い、待ち受けのソケットは開かない
	ua, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	c, err := net.DialUDP("udp", nil, ua)
	if err != nil {
		return nil, err
	}
	return &udpTransport{c: c}, nil
}

type udpTransport struct {
	c *net.UDPConn
}

func (t *udpTransport) send(b []byte) error {
	_, err := t.c.Write(b)
	return err
}

func (t *udpTransport) recv() ([]byte, error) {
	buf := make([]byte, 65535)
	for {
		n, err := t.c.Read(buf)
		if errors.Is(err, net.ErrClosed) {
			return nil, err
		}
		if err != nil {
			// 相手のポートが閉じていると、Windows では ICMP を受けて読み出しが失敗する。そのまま読み続ける
			continue
		}
		return append([]byte{}, buf[:n]...), nil
	}
}

func (t *udpTransport) close() error { return t.c.Close() }

// tcpTransport は、パケットの前に 2 バイトの長さを付けて送る
type tcpTransport struct {
	c  net.Conn
	r  *bufio.Reader
	mu sync.Mutex
}

func (t *tcpTransport) send(b []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, err := t.c.Write(append(binary.BigEndian.AppendUint16(nil, uint16(len(b))), b...))
	return err
}

func (t *tcpTransport) recv() ([]byte, error) {
	var l [2]byte
	if _, err := io.ReadFull(t.r, l[:]); err != nil {
		return nil, err
	}
	b := make([]byte, binary.BigEndian.Uint16(l[:]))
	if _, err := io.ReadFull(t.r, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (t *tcpTransport) close() error { return t.c.Close() }
