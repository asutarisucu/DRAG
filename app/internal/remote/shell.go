package remote

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Shell は PTY 付きのチャネルで動くログインシェル。
// 前面のコマンドを /proc から調べるために、開くときにシェルの PID を知っておく
type Shell struct {
	sess *ssh.Session
	in   io.WriteCloser
	PID  int

	closeOnce sync.Once
}

// OpenShell は cwd でログインシェルを開く。cwd が空ならホーム。
// 出力は onOutput に、終わったら onExit に渡す。どちらも別のゴルーチンから呼ぶ
func (s *Server) OpenShell(cols, rows int, cwd string, onOutput func([]byte), onExit func()) (*Shell, error) {
	sess, err := s.ssh.NewSession()
	if err != nil {
		return nil, err
	}
	if err := sess.RequestPty("xterm-256color", rows, cols, nil); err != nil {
		sess.Close()
		return nil, err
	}
	in, _ := sess.StdinPipe()
	out, _ := sess.StdoutPipe()
	// sh を挟んで PID を1行出させ、同じ PID のままログインシェルに exec する。
	// 開くフォルダは、引用符の扱いがシェルごとに違うので、コマンドに埋めずに標準入力の1行目で渡す
	nonce := rand.Text()[:10]
	script := `IFS= read -r d; cd "${d:-$HOME}"; echo "DRAG-` + nonce + ` $$"; exec "$SHELL" -l`
	if err := sess.Start("exec /bin/sh -c '" + script + "'"); err != nil {
		sess.Close()
		return nil, err
	}
	// Start より前に書くと sshd に捨てられる
	fmt.Fprintf(in, "%s\n", cwd)

	sh := &Shell{sess: sess, in: in}
	re := regexp.MustCompile(`DRAG-` + nonce + ` (\d+)\r?\n`)
	found := make(chan struct{})
	go func() {
		defer func() {
			sess.Wait()
			onExit()
		}()
		var pre []byte
		b := make([]byte, 32*1024)
		for {
			n, err := out.Read(b)
			if sh.PID == 0 {
				// PID の行より前は、入力した cwd のエコーとログイン前の出力なので捨てる
				pre = append(pre, b[:n]...)
				if m := re.FindSubmatchIndex(pre); m != nil {
					sh.PID, _ = strconv.Atoi(string(pre[m[2]:m[3]]))
					close(found)
					if rest := pre[m[1]:]; len(rest) > 0 {
						onOutput(bytes.Clone(rest))
					}
				}
			} else if n > 0 {
				onOutput(bytes.Clone(b[:n]))
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-found:
		return sh, nil
	case <-time.After(10 * time.Second):
		sess.Close()
		return nil, fmt.Errorf("シェルが起動しません")
	}
}

func (sh *Shell) Write(p []byte) (int, error) {
	return sh.in.Write(p)
}

func (sh *Shell) Resize(cols, rows int) error {
	return sh.sess.WindowChange(rows, cols)
}

func (sh *Shell) Close() {
	sh.closeOnce.Do(func() { sh.sess.Close() })
}
