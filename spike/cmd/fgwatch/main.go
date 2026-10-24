// fgwatch は PC アプリと同じく SSH のチャネルだけを使い、ターミナルの前面で動いているコマンドを調べる。
// サーバーには何も置かない。確かめること
//   - シェルの PID を、シェルを開くときに知れるか
//   - 別のチャネルから /proc を読んで、前面のコマンドと作業中のフォルダが分かるか
//   - コマンドを打ってから分かるまでの時間
//   - ログインシェルが bash・zsh・fish のどれでも同じ手順で動くか
package main

import (
	"bufio"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

var (
	addr     = flag.String("addr", "127.0.0.1:2222", "SSH の接続先")
	pass     = flag.String("pass", "drag", "パスワード")
	interval = flag.Duration("interval", 250*time.Millisecond, "前面のコマンドを調べる間隔")
)

var failed bool

func check(name string, ok bool, detail string) {
	mark := "OK  "
	if !ok {
		mark = "NG  "
		failed = true
	}
	fmt.Printf("%s %s  %s\n", mark, name, detail)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func dial(user string) (*ssh.Client, error) {
	return ssh.Dial("tcp", *addr, &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(*pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
}

// shell は PTY 付きのチャネルで動くログインシェル
type shell struct {
	in  io.Writer
	pid int

	mu  sync.Mutex
	out []byte // PID の行より後の出力
}

// openShell は cwd でログインシェルを開き、その PID を知る。
// sh を挟んで PID を1行出させ、同じ PID のままログインシェルに exec する。
// cwd は引用符の扱いがシェルごとに違うので、コマンドに埋めずに標準入力で渡す
func openShell(c *ssh.Client, cwd string) (*shell, time.Duration, error) {
	t := time.Now()
	s, err := c.NewSession()
	if err != nil {
		return nil, 0, err
	}
	if err := s.RequestPty("xterm-256color", 30, 120, nil); err != nil {
		return nil, 0, err
	}
	in, _ := s.StdinPipe()
	out, _ := s.StdoutPipe()
	nonce := rand.Text()[:8]
	script := `IFS= read -r d; cd "${d:-$HOME}"; echo "DRAG-` + nonce + ` $$"; exec "$SHELL" -l`
	if err := s.Start("exec /bin/sh -c '" + script + "'"); err != nil {
		return nil, 0, err
	}
	fmt.Fprintf(in, "%s\n", cwd)

	sh := &shell{in: in}
	re := regexp.MustCompile(`DRAG-` + nonce + ` (\d+)\r?\n`)
	found := make(chan struct{})
	go func() {
		var pre []byte
		p := make([]byte, 32*1024)
		for {
			n, err := out.Read(p)
			sh.mu.Lock()
			if sh.pid == 0 {
				// PID の行より前は、入力した cwd のエコーとログイン時の出力なので捨てる
				pre = append(pre, p[:n]...)
				if m := re.FindSubmatchIndex(pre); m != nil {
					sh.pid, _ = strconv.Atoi(string(pre[m[2]:m[3]]))
					sh.out = append(sh.out, pre[m[1]:]...)
					close(found)
				}
			} else {
				sh.out = append(sh.out, p[:n]...)
			}
			sh.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-found:
		return sh, time.Since(t), nil
	case <-time.After(5 * time.Second):
		return nil, 0, fmt.Errorf("シェルの PID が出てこない")
	}
}

func (sh *shell) output() string {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return string(sh.out)
}

// waitOutput は、出力に want が出るまで待つ
func (sh *shell) waitOutput(want string) bool {
	for t := time.Now(); time.Since(t) < 5*time.Second; time.Sleep(10 * time.Millisecond) {
		if strings.Contains(sh.output(), want) {
			return true
		}
	}
	return false
}

// monitor は PTY を付けずに開いた sh。PC アプリから1行ずつコマンドを送って /proc を読む
type monitor struct {
	in  io.Writer
	out *bufio.Reader
}

const endMark = "__DRAG_END__"

func newMonitor(c *ssh.Client) (*monitor, error) {
	s, err := c.NewSession()
	if err != nil {
		return nil, err
	}
	in, _ := s.StdinPipe()
	out, _ := s.StdoutPipe()
	if err := s.Start("exec /bin/sh"); err != nil {
		return nil, err
	}
	return &monitor{in: in, out: bufio.NewReader(out)}, nil
}

func (m *monitor) run(script string) (string, error) {
	if _, err := fmt.Fprintf(m.in, "%s\necho %s\n", script, endMark); err != nil {
		return "", err
	}
	var b strings.Builder
	for {
		line, err := m.out.ReadString('\n')
		if err != nil {
			return "", err
		}
		if line == endMark+"\n" {
			return b.String(), nil
		}
		b.WriteString(line)
	}
}

// tpgid は、シェルの端末で前面にいるプロセスグループの番号を返す。
// read と echo は sh の組み込みなので、調べるたびにプロセスは増えない
func (m *monitor) tpgid(pid int) (int, error) {
	out, err := m.run(fmt.Sprintf(`read -r l < /proc/%d/stat && echo "$l"`, pid))
	if err != nil {
		return 0, err
	}
	// 2つ目の項目 (comm) は括弧の中に空白や括弧を含みうるので、最後の ')' の後ろから数える。
	// state ppid pgrp session tty_nr tpgid の順
	f := strings.Fields(out[strings.LastIndexByte(out, ')')+1:])
	if len(f) < 6 {
		return 0, fmt.Errorf("stat を読めない: %q", out)
	}
	return strconv.Atoi(f[5])
}

// foreground は前面のコマンドを返す。シェル自身が前面なら空
func (m *monitor) foreground(pid int) (string, error) {
	pg, err := m.tpgid(pid)
	if err != nil || pg <= 0 || pg == pid {
		return "", err
	}
	out, err := m.run(fmt.Sprintf(`tr '\0' ' ' < /proc/%d/cmdline; echo`, pg))
	return strings.TrimSpace(out), err
}

// expect は keys を打ってから、前面のコマンドが want で始まるまでの時間を測る。want が空ならシェルに戻るまで
func expect(m *monitor, sh *shell, name, keys, want string) {
	t := time.Now()
	if keys != "" {
		io.WriteString(sh.in, keys)
	}
	got := ""
	for time.Since(t) < 5*time.Second {
		var err error
		got, err = m.foreground(sh.pid)
		if err != nil {
			check(name, false, err.Error())
			return
		}
		if (want == "" && got == "") || (want != "" && strings.HasPrefix(got, want)) {
			if got == "" {
				got = "(シェル)"
			}
			check(name, true, fmt.Sprintf("%s  %v", got, time.Since(t).Round(time.Millisecond)))
			return
		}
		time.Sleep(*interval)
	}
	check(name, false, fmt.Sprintf("前面は %q のまま", got))
}

func cwdOf(m *monitor, pid int) string {
	out, _ := m.run(fmt.Sprintf("readlink /proc/%d/cwd", pid))
	return strings.TrimSpace(out)
}

func main() {
	flag.Parse()

	fmt.Println("== bash (asuta)")
	c, err := dial("asuta")
	must(err)
	m, err := newMonitor(c)
	must(err)
	t := time.Now()
	sh, took, err := openShell(c, "")
	must(err)
	check("シェルの PID", sh.pid > 0, fmt.Sprintf("pid %d  PID が分かるまで %v", sh.pid, took.Round(time.Millisecond)))
	io.WriteString(sh.in, "echo READ\"\"Y\r")
	check("ログインシェルがコマンドを受け付ける", sh.waitOutput("READY"), time.Since(t).Round(time.Millisecond).String())

	var rtt time.Duration
	for range 20 {
		t := time.Now()
		_, err := m.tpgid(sh.pid)
		must(err)
		rtt += time.Since(t)
	}
	fmt.Printf("     stat を1回読む往復  %v (20回の平均)\n", (rtt / 20).Round(time.Microsecond))

	expect(m, sh, "何も動いていない", "", "")
	expect(m, sh, "sleep", "sleep 2\r", "sleep 2")
	expect(m, sh, "sleep が終わる", "", "")
	expect(m, sh, "パイプ", "sleep 30 | cat\r", "sleep 30")
	expect(m, sh, "Ctrl+C", "\x03", "")
	expect(m, sh, "vi", "vi /tmp/fg.txt\r", "vi /tmp/fg.txt")
	expect(m, sh, "vi を閉じる", "\x1b:q!\r", "")
	expect(m, sh, "screen", "screen -S fg\r", "screen -S fg")
	expect(m, sh, "screen をデタッチ", "\x01d", "")
	expect(m, sh, "tmux", "tmux new -s fg\r", "tmux new -s fg")
	expect(m, sh, "tmux をデタッチ", "\x02d", "")
	expect(m, sh, "ssh", "ssh -o StrictHostKeyChecking=no asuta@localhost\r", "ssh -o StrictHostKeyChecking=no asuta@localhost")
	time.Sleep(time.Second)
	expect(m, sh, "ssh のパスワード入力で Ctrl+C", "\x03", "")
	expect(m, sh, "bash の中の bash", "bash\r", "bash")
	expect(m, sh, "中の bash を抜ける", "exit\r", "")
	expect(m, sh, "sudo", "sudo -s\r", "sudo -s")
	time.Sleep(500 * time.Millisecond)
	expect(m, sh, "sudo のパスワード入力で Ctrl+C", "\x03", "")
	out, _ := m.run("screen -ls; tmux ls; screen -S fg -X quit; tmux kill-server")
	check("デタッチしたセッションが残る", strings.Contains(out, ".fg\t") && strings.Contains(out, "fg: 1 windows"), strings.Join(strings.Fields(out), " "))

	// 切り出し。前面のコマンドはそのチャネルに残し、同じフォルダで新しいシェルを開く
	io.WriteString(sh.in, "cd /var/log\r")
	time.Sleep(300 * time.Millisecond)
	expect(m, sh, "切り出すコマンド", "sleep 30\r", "sleep 30")
	dir := cwdOf(m, sh.pid)
	check("作業中のフォルダ", dir == "/var/log", dir)
	t = time.Now()
	sh2, _, err := openShell(c, dir)
	must(err)
	io.WriteString(sh2.in, "pwd\r")
	check("新しいシェルが同じフォルダで開く", sh2.waitOutput("/var/log\r\n"), fmt.Sprintf("pwd が返るまで %v", time.Since(t).Round(time.Millisecond)))
	expect(m, sh, "元のチャネルではまだ動いている", "", "sleep 30")
	io.WriteString(sh.in, "\x03")
	c.Close()

	for _, u := range []string{"zuser", "fuser"} {
		fmt.Printf("== %s\n", u)
		c, err := dial(u)
		must(err)
		m, err := newMonitor(c)
		must(err)
		t := time.Now()
		sh, took, err := openShell(c, "/tmp")
		must(err)
		login, _ := m.run(fmt.Sprintf("tr '\\0' ' ' < /proc/%d/cmdline; echo", sh.pid))
		check("シェルの PID", sh.pid > 0, fmt.Sprintf("pid %d  %s  PID が分かるまで %v", sh.pid, strings.TrimSpace(login), took.Round(time.Millisecond)))
		io.WriteString(sh.in, "echo READ\"\"Y\r")
		check("ログインシェルがコマンドを受け付ける", sh.waitOutput("READY"), time.Since(t).Round(time.Millisecond).String())
		check("作業中のフォルダ", cwdOf(m, sh.pid) == "/tmp", cwdOf(m, sh.pid))
		expect(m, sh, "何も動いていない", "", "")
		expect(m, sh, "sleep", "sleep 2\r", "sleep 2")
		expect(m, sh, "sleep が終わる", "", "")
		expect(m, sh, "vi", "vi /tmp/fg.txt\r", "vi /tmp/fg.txt")
		expect(m, sh, "vi を閉じる", "\x1b:q!\r", "")
		c.Close()
	}

	if failed {
		os.Exit(1)
	}
}
