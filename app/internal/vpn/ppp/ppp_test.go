package ppp

import (
	"encoding/binary"
	"sync"
	"testing"
)

// testConn は、送ったフレームを貯める PPP。サーバーのふりをしてフレームを渡して試す
func testConn(cfg Config) (*Conn, func() [][]byte) {
	var mu sync.Mutex
	var sent [][]byte
	c := New(cfg, func(f []byte) error {
		mu.Lock()
		sent = append(sent, append([]byte{}, f...))
		mu.Unlock()
		return nil
	})
	return c, func() [][]byte {
		mu.Lock()
		defer mu.Unlock()
		out := sent
		sent = nil
		return out
	}
}

func frame(proto uint16, p []byte) []byte {
	return append(binary.BigEndian.AppendUint16([]byte{0xff, 0x03}, proto), p...)
}

// lcpReply は、送ったものから、id の Configure-Request への LCP の返事のコードを探す
func lcpReply(sent [][]byte, id byte) byte {
	for _, f := range sent {
		if len(f) >= 8 && binary.BigEndian.Uint16(f[2:4]) == protoLCP && f[5] == id {
			return f[4]
		}
	}
	return 0
}

func sentProto(sent [][]byte, proto uint16) bool {
	for _, f := range sent {
		if len(f) >= 4 && binary.BigEndian.Uint16(f[2:4]) == proto {
			return true
		}
	}
	return false
}

// MS-CHAPv2 を求めるときは、PAP、CHAP-MD5、認証なしを受け入れず、パスワードを送らない
func TestRequireMSCHAPv2(t *testing.T) {
	for _, c := range []struct {
		name string
		auth []byte
	}{
		{"PAP", opt(optAuth, []byte{0xc0, 0x23})},
		{"CHAP-MD5", opt(optAuth, []byte{0xc2, 0x23, 0x05})},
		{"認証なし", nil},
	} {
		conn, sent := testConn(Config{User: "u", Pass: "secret", RequireMSCHAPv2: true})
		conn.lcpOurs = true
		conn.handle(frame(protoLCP, packet(confReq, 7, c.auth)))
		out := sent()
		if code := lcpReply(out, 7); code != confNak {
			t.Errorf("%s: 返事 %d", c.name, code)
		}
		if sentProto(out, protoPAP) || conn.authDone {
			t.Errorf("%s: 認証を進めた", c.name)
		}
		for range maxAuthNaks {
			conn.handle(frame(protoLCP, packet(confReq, 8, c.auth)))
		}
		select {
		case <-conn.Done():
		default:
			t.Errorf("%s: 求め直しをやめない", c.name)
		}
	}
	// MS-CHAPv2 なら受け入れる
	conn, sent := testConn(Config{User: "u", Pass: "secret", RequireMSCHAPv2: true})
	conn.handle(frame(protoLCP, packet(confReq, 9, opt(optAuth, []byte{0xc2, 0x23, 0x81}))))
	if code := lcpReply(sent(), 9); code != confAck {
		t.Errorf("MS-CHAPv2: 返事 %d", code)
	}
}

// SSTP は TLS でサーバーを確かめるので、PAP も受け入れる
func TestPAPWithoutRequire(t *testing.T) {
	conn, sent := testConn(Config{User: "u", Pass: "secret"})
	conn.lcpOurs = true
	conn.handle(frame(protoLCP, packet(confReq, 7, opt(optAuth, []byte{0xc0, 0x23}))))
	out := sent()
	if lcpReply(out, 7) != confAck || !sentProto(out, protoPAP) {
		t.Fatalf("%x", out)
	}
}

// 長さの欄が 4 より小さいパケットは、範囲の外を読まずに捨てる
func TestShortLengthIsDropped(t *testing.T) {
	for _, proto := range []uint16{protoLCP, protoCHAP, protoIPCP} {
		conn, _ := testConn(Config{})
		conn.authDone = true
		conn.handleLocked(frame(proto, []byte{confReq, 1, 0, 0}))
		select {
		case <-conn.Done():
			t.Errorf("%04x: %v", proto, conn.err)
		default:
		}
	}
}

// サーバーが IP-Address を断ったら、panic せずに接続を失敗にする
func TestIPAddressRejected(t *testing.T) {
	conn, _ := testConn(Config{})
	conn.authDone = true
	conn.sendIPCPReq()
	conn.handleLocked(frame(protoIPCP, packet(confRej, conn.ipcpReq[1], opt(optIPAddr, make([]byte, 4)))))
	select {
	case <-conn.Done():
	default:
		t.Fatal("IP-Address を断られても続けた")
	}
	// IP-Address が無いまま両方がまとまっても落ちない
	conn, _ = testConn(Config{})
	delete(conn.ipcpOpts, optIPAddr)
	conn.ipcpOurs, conn.ipcpTheirs = true, true
	conn.ipcpUp()
	if conn.err == nil {
		t.Fatal("アドレスの無いまま開いた")
	}
}
