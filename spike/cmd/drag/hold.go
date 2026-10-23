//go:build linux

package main

import (
	"encoding/binary"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/creack/pty"
)

// 保持役は1つのセッションのシェルと PTY を持つ。エージェントとは別のプロセスなので、
// エージェントを入れ替えたり落としたりしても、シェルは動き続ける

// つなぎ直したクライアントに最初に送る、直近の出力の量
const replaySize = 512 << 10

type holder struct {
	pty     *os.File
	mu      sync.Mutex
	buf     []byte
	clients map[*holdClient]struct{}
	writers sync.WaitGroup
}

type holdClient struct {
	conn net.Conn
	out  chan []byte
}

func runHold(id string) error {
	sock := filepath.Join(runDir(), id+".sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	defer os.Remove(sock)

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-l")
	cmd.Dir = os.Getenv("HOME")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 80, Rows: 24})
	if err != nil {
		return err
	}

	h := &holder{pty: f, clients: map[*holdClient]struct{}{}}
	go h.accept(ln)
	h.pump()

	code := 0
	if err := cmd.Wait(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return err
		}
		code = ee.ExitCode()
	}
	ln.Close()
	h.exit(code)
	return nil
}

// PTY の出力を読み、全クライアントに配る。シェルが終わると読めなくなって戻る
func (h *holder) pump() {
	b := make([]byte, 32<<10)
	for {
		n, err := h.pty.Read(b)
		if n > 0 {
			h.broadcast(b[:n])
		}
		if err != nil {
			return
		}
	}
}

func (h *holder) broadcast(p []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.buf = append(h.buf, p...)
	if over := len(h.buf) - replaySize; over > 0 {
		h.buf = append(h.buf[:0], h.buf[over:]...)
	}
	f := encodeFrame(frameOutput, p)
	for c := range h.clients {
		select {
		case c.out <- f:
		default:
			// 受け取りが追いつかないクライアントは切る。シェルの側を止めない
			h.drop(c)
		}
	}
}

func (h *holder) accept(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		c := &holdClient{conn: conn, out: make(chan []byte, 256)}
		h.mu.Lock()
		c.out <- encodeFrame(frameOutput, h.buf)
		h.clients[c] = struct{}{}
		h.mu.Unlock()
		h.writers.Add(1)
		go h.writeLoop(c)
		go h.readLoop(c)
	}
}

func (h *holder) writeLoop(c *holdClient) {
	defer h.writers.Done()
	defer c.conn.Close()
	for f := range c.out {
		if _, err := c.conn.Write(f); err != nil {
			h.remove(c)
			for range c.out {
			}
			return
		}
	}
}

func (h *holder) readLoop(c *holdClient) {
	for {
		typ, p, err := readFrame(c.conn)
		if err != nil {
			h.remove(c)
			return
		}
		switch typ {
		case frameInput:
			h.pty.Write(p)
		case frameResize:
			if len(p) == 4 {
				pty.Setsize(h.pty, &pty.Winsize{
					Cols: binary.BigEndian.Uint16(p[0:]),
					Rows: binary.BigEndian.Uint16(p[2:]),
				})
			}
		}
	}
}

func (h *holder) remove(c *holdClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.drop(c)
}

// h.mu を持った状態で呼ぶ
func (h *holder) drop(c *holdClient) {
	if _, ok := h.clients[c]; !ok {
		return
	}
	delete(h.clients, c)
	close(c.out)
	c.conn.Close()
}

// シェルの終了をクライアントに知らせ、送り終わるのを少しだけ待つ
func (h *holder) exit(code int) {
	var p [4]byte
	binary.BigEndian.PutUint32(p[:], uint32(int32(code)))
	h.mu.Lock()
	for c := range h.clients {
		select {
		case c.out <- encodeFrame(frameExit, p[:]):
		default:
		}
		delete(h.clients, c)
		close(c.out)
	}
	h.mu.Unlock()

	done := make(chan struct{})
	go func() { h.writers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}
