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

// replayLimit は、ウィンドウへ移すときに渡し直すため、シェルごとに残しておく出力の量
const replayLimit = 256 * 1024

// ShellEvents はシェルの出来事の受け手。どれも別のゴルーチンから呼ぶ
type ShellEvents struct {
	// Output は出力。end はこのシェルの最初からの通算の位置で、この出力の末尾を指す
	Output func(p []byte, end int64)
	// Foreground は前面のコマンドが変わったとき。シェル自身に戻ったら空
	Foreground func(cmdline string)
	Exit       func()
}

// Shell は PTY 付きのチャネルで動くログインシェル。
// 前面のコマンドを /proc から調べるために、開くときにシェルの PID を知っておく
type Shell struct {
	srv  *Server
	sess *ssh.Session
	in   io.WriteCloser
	PID  int
	ev   ShellEvents

	mu      sync.Mutex
	cols    int
	rows    int
	buf     []byte // 直近の出力
	total   int64  // 最初からの通算の出力の量
	idleOff int64  // 最後にシェル自身が前面だったときの total
	fg      string

	// monitor のゴルーチンだけが触る
	tpgid  int
	noProc bool

	writeMu   sync.Mutex
	closeOnce sync.Once
}

// OpenShell は cwd でログインシェルを開く。cwd が空ならホーム
func (s *Server) OpenShell(cols, rows int, cwd string, ev ShellEvents) (*Shell, error) {
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

	sh := &Shell{srv: s, sess: sess, in: in, ev: ev, cols: cols, rows: rows}
	re := regexp.MustCompile(`DRAG-` + nonce + ` (\d+)\r?\n`)
	found := make(chan struct{})
	go func() {
		defer func() {
			sess.Wait()
			if s.mon != nil {
				s.mon.remove(sh)
			}
			ev.Exit()
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
					sh.output(pre[m[1]:])
				}
			} else {
				sh.output(b[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-found:
		if s.mon != nil {
			s.mon.add(sh)
		}
		return sh, nil
	case <-time.After(10 * time.Second):
		sess.Close()
		return nil, fmt.Errorf("シェルが起動しません")
	}
}

func (sh *Shell) output(p []byte) {
	if len(p) == 0 {
		return
	}
	sh.mu.Lock()
	sh.buf = append(sh.buf, p...)
	if over := len(sh.buf) - replayLimit; over > 0 {
		sh.buf = append(sh.buf[:0:0], sh.buf[over:]...)
	}
	sh.total += int64(len(p))
	end := sh.total
	sh.mu.Unlock()
	sh.ev.Output(bytes.Clone(p), end)
}

// markIdle は、シェル自身が前面だったときの出力の位置を覚える。次に打ったコマンドの行はここから後ろにある
func (sh *Shell) markIdle() {
	sh.mu.Lock()
	sh.idleOff = sh.total
	sh.mu.Unlock()
}

func (sh *Shell) setForeground(cmdline string) {
	sh.mu.Lock()
	if cmdline == "" {
		sh.idleOff = sh.total
	}
	sh.fg = cmdline
	sh.mu.Unlock()
	sh.ev.Foreground(cmdline)
}

// Foreground は前面のコマンド。シェル自身なら空
func (sh *Shell) Foreground() string {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return sh.fg
}

// Replay は残しておいた出力と、その末尾の通算の位置を返す。
// fromIdle なら、最後にシェル自身が前面だった時点の行の頭から返す。
// ウィンドウへ移したコマンドの、打った行から後ろだけを見せるのに使う
func (sh *Shell) Replay(fromIdle bool) ([]byte, int64) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	start := 0
	if fromIdle {
		base := sh.total - int64(len(sh.buf))
		if i := sh.idleOff - base; i > 0 && i <= int64(len(sh.buf)) {
			start = bytes.LastIndexByte(sh.buf[:i], '\n') + 1
		}
	}
	return bytes.Clone(sh.buf[start:]), sh.total
}

// Cwd はシェルの作業中のフォルダ
func (sh *Shell) Cwd() (string, error) {
	sess, err := sh.srv.ssh.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	out, err := sess.Output(fmt.Sprintf("exec /bin/sh -c 'readlink /proc/%d/cwd'", sh.PID))
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(out)), nil
}

// Write はシェルへ書く。Wails は画面からの呼び出しを並行して動かすので、ここで1つずつにする。
// x/crypto/ssh のチャネルの書き込みは送る前のバッファを共有していて、重なると壊れたパケットになり、
// sshd が「unexpected bytes remain after decoding」で接続ごと切る
func (sh *Shell) Write(p []byte) (int, error) {
	sh.writeMu.Lock()
	defer sh.writeMu.Unlock()
	return sh.in.Write(p)
}

func (sh *Shell) Resize(cols, rows int) error {
	sh.mu.Lock()
	sh.cols, sh.rows = cols, rows
	sh.mu.Unlock()
	return sh.sess.WindowChange(rows, cols)
}

// Size は PTY の桁数と行数
func (sh *Shell) Size() (int, int) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return sh.cols, sh.rows
}

func (sh *Shell) Close() {
	sh.closeOnce.Do(func() { sh.sess.Close() })
}
