package remote

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"regexp"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// fakeServer は、exec のコマンドを受けたら reply が返す行を書いて閉じる SSH のサーバー。
// 悪意のあるサーバーの返事を試すのに使う
func fakeServer(t *testing.T, reply func(cmd string) string) *Server {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := ssh.NewSignerFromKey(priv)
	conf := &ssh.ServerConfig{NoClientAuth: true}
	conf.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		nc, err := ln.Accept()
		if err != nil {
			return
		}
		_, chans, reqs, err := ssh.NewServerConn(nc, conf)
		if err != nil {
			return
		}
		go ssh.DiscardRequests(reqs)
		for nch := range chans {
			ch, creqs, err := nch.Accept()
			if err != nil {
				continue
			}
			go func() {
				for r := range creqs {
					r.Reply(r.Type == "pty-req" || r.Type == "exec", nil)
					if r.Type == "exec" {
						var p struct{ Cmd string }
						ssh.Unmarshal(r.Payload, &p)
						ch.Write([]byte(reply(p.Cmd)))
						ch.SendRequest("exit-status", false, ssh.Marshal(struct{ N uint32 }{0}))
						ch.Close()
					}
				}
			}()
		}
	}()
	sc, err := ssh.Dial("tcp", ln.Addr().String(), &ssh.ClientConfig{User: "x", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sc.Close() })
	return &Server{link: &link{ssh: sc}}
}

// PID の行の値が 0 でも、知らせを2回閉じて落ちたりせず、開けないと返す
func TestOpenShellRejectsZeroPID(t *testing.T) {
	re := regexp.MustCompile(`DRAG-\w+`)
	s := fakeServer(t, func(cmd string) string { return re.FindString(cmd) + " 0\r\n" })
	done := make(chan struct{})
	_, err := s.OpenShell(80, 24, "", ShellEvents{Output: func([]byte, int64) {}, Foreground: func(string) {}, Exit: func(bool) { close(done) }})
	if err == nil {
		t.Fatal("PID 0 のシェルを開いた")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("読む側が終わらない")
	}
}
