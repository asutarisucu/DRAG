// wgpeer は、画面の試験で使う WireGuard の相手。127.0.0.1 の UDP で待ち受け、
// VPN の中の 10.99.0.1:22 で受けた TCP を、-forward の先 (検証用のサーバー) へ渡す。
// 起動すると、DRAG に読ませる .conf を -conf に書く。止めるまで動く
//
//	go run . -conf ../../app/frontend/e2e/out/wg.conf -forward 127.0.0.1:2222
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"

	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

func key() (priv, pub []byte) {
	priv = make([]byte, 32)
	rand.Read(priv)
	pub, err := curve25519.X25519(priv, curve25519.Basepoint)
	if err != nil {
		log.Fatal(err)
	}
	return priv, pub
}

func main() {
	confPath := flag.String("conf", "wg.conf", "DRAG に読ませる .conf を書く先")
	forward := flag.String("forward", "127.0.0.1:2222", "VPN の中の 10.99.0.1:22 で受けたものを渡す先")
	flag.Parse()

	srvPriv, srvPub := key()
	cliPriv, cliPub := key()
	tdev, tnet, err := netstack.CreateNetTUN([]netip.Addr{netip.MustParseAddr("10.99.0.1")}, nil, 1420)
	if err != nil {
		log.Fatal(err)
	}
	bind := &loopBind{}
	dev := device.NewDevice(tdev, bind, device.NewLogger(device.LogLevelError, "wgpeer: "))
	if err := dev.IpcSet(fmt.Sprintf("private_key=%s\npublic_key=%s\nallowed_ip=10.99.0.2/32\n", hex.EncodeToString(srvPriv), hex.EncodeToString(cliPub))); err != nil {
		log.Fatal(err)
	}
	if err := dev.Up(); err != nil {
		log.Fatal(err)
	}
	l, err := tnet.ListenTCPAddrPort(netip.MustParseAddrPort("10.99.0.1:22"))
	if err != nil {
		log.Fatal(err)
	}
	b64 := base64.StdEncoding.EncodeToString
	conf := fmt.Sprintf("[Interface]\nPrivateKey = %s\nAddress = 10.99.0.2/32\n\n[Peer]\nPublicKey = %s\nEndpoint = 127.0.0.1:%d\nAllowedIPs = 10.99.0.0/24\n",
		b64(cliPriv), b64(srvPub), bind.port)
	if err := os.WriteFile(*confPath, []byte(conf), 0o600); err != nil {
		log.Fatal(err)
	}
	log.Printf("127.0.0.1:%d で待ち受け、%s を書きました", bind.port, *confPath)
	for {
		c, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			defer c.Close()
			up, err := net.Dial("tcp", *forward)
			if err != nil {
				return
			}
			defer up.Close()
			go io.Copy(up, c)
			io.Copy(c, up)
		}()
	}
}

// loopBind は 127.0.0.1 だけで待ち受ける conn.Bind
type loopBind struct {
	c    *net.UDPConn
	port uint16
}

type endpoint netip.AddrPort

func (e endpoint) ClearSrc()           {}
func (e endpoint) SrcToString() string { return "" }
func (e endpoint) DstToString() string { return netip.AddrPort(e).String() }
func (e endpoint) DstToBytes() []byte {
	b, _ := netip.AddrPort(e).MarshalBinary()
	return b
}
func (e endpoint) DstIP() netip.Addr { return netip.AddrPort(e).Addr() }
func (e endpoint) SrcIP() netip.Addr { return netip.Addr{} }

func (b *loopBind) Open(uint16) ([]conn.ReceiveFunc, uint16, error) {
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, 0, err
	}
	b.c, b.port = c, uint16(c.LocalAddr().(*net.UDPAddr).Port)
	fn := func(packets [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
		n, from, err := c.ReadFromUDPAddrPort(packets[0])
		if err != nil {
			return 0, err
		}
		sizes[0], eps[0] = n, endpoint(from)
		return 1, nil
	}
	return []conn.ReceiveFunc{fn}, b.port, nil
}
func (b *loopBind) Close() error {
	if b.c != nil {
		return b.c.Close()
	}
	return nil
}
func (b *loopBind) SetMark(uint32) error { return nil }
func (b *loopBind) BatchSize() int       { return 1 }
func (b *loopBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	ap, err := netip.ParseAddrPort(s)
	return endpoint(ap), err
}
func (b *loopBind) Send(bufs [][]byte, ep conn.Endpoint) error {
	for _, buf := range bufs {
		if _, err := b.c.WriteToUDPAddrPort(buf, netip.AddrPort(ep.(endpoint))); err != nil {
			return err
		}
	}
	return nil
}
