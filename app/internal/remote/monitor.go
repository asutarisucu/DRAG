package remote

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// monitor は、開いているシェルの前面のコマンドを見張る。
// PTY を付けない sh を1つ開いておき、0.25 秒おきに全部のシェルの /proc/<PID>/stat をまとめて読む。
// read と echo は sh の組み込みなので、見張るだけならサーバーでプロセスは増えない
type monitor struct {
	in  io.Writer
	out *bufio.Reader

	mu     sync.Mutex
	shells map[*Shell]struct{}
}

const monitorEnd = "__DRAG_END__"

func (s *Server) startMonitor() error {
	sess, err := s.ssh.NewSession()
	if err != nil {
		return err
	}
	in, _ := sess.StdinPipe()
	out, _ := sess.StdoutPipe()
	if err := sess.Start("exec /bin/sh"); err != nil {
		sess.Close()
		return err
	}
	s.mon = &monitor{in: in, out: bufio.NewReader(out), shells: map[*Shell]struct{}{}}
	go func() {
		defer sess.Close()
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			if s.isClosed() || s.mon.poll() != nil {
				return
			}
		}
	}()
	return nil
}

func (m *monitor) add(sh *Shell) {
	m.mu.Lock()
	m.shells[sh] = struct{}{}
	m.mu.Unlock()
}

func (m *monitor) remove(sh *Shell) {
	m.mu.Lock()
	delete(m.shells, sh)
	m.mu.Unlock()
}

func (m *monitor) run(script string) ([]string, error) {
	if _, err := fmt.Fprintf(m.in, "%s\necho %s\n", script, monitorEnd); err != nil {
		return nil, err
	}
	var lines []string
	for {
		l, err := m.out.ReadString('\n')
		if err != nil {
			return nil, err
		}
		l = strings.TrimSuffix(l, "\n")
		if l == monitorEnd {
			return lines, nil
		}
		lines = append(lines, l)
	}
}

// poll は前面のプロセスグループの番号を読み、変わったシェルだけコマンドを読み直して知らせる
func (m *monitor) poll() error {
	m.mu.Lock()
	shells := make(map[int]*Shell, len(m.shells))
	for sh := range m.shells {
		if !sh.noProc {
			shells[sh.PID] = sh
		}
	}
	m.mu.Unlock()
	if len(shells) == 0 {
		return nil
	}
	var b strings.Builder
	for pid := range shells {
		fmt.Fprintf(&b, "if read -r l < /proc/%d/stat; then echo \"%d $l\"; else echo \"%d -\"; fi 2>/dev/null\n", pid, pid, pid)
	}
	lines, err := m.run(b.String())
	if err != nil {
		return err
	}
	changed := map[*Shell]int{}
	for _, l := range lines {
		head, rest, _ := strings.Cut(l, " ")
		pid, _ := strconv.Atoi(head)
		sh := shells[pid]
		if sh == nil {
			continue
		}
		if rest == "-" {
			// /proc が無いサーバーでは見張れない
			sh.noProc = true
			continue
		}
		// 2つ目の項目 (comm) は括弧の中に空白を含みうるので、最後の ')' の後ろから数える。
		// state ppid pgrp session tty_nr tpgid の順
		f := strings.Fields(rest[strings.LastIndexByte(rest, ')')+1:])
		if len(f) < 6 {
			continue
		}
		tpgid, _ := strconv.Atoi(f[5])
		if tpgid <= 0 || tpgid == pid {
			tpgid = 0
			sh.markIdle()
		}
		if tpgid != sh.tpgid {
			sh.tpgid = tpgid
			changed[sh] = tpgid
		}
	}
	if len(changed) == 0 {
		return nil
	}
	b.Reset()
	for _, pg := range changed {
		if pg != 0 {
			fmt.Fprintf(&b, "printf '%d '; tr '\\0' ' ' < /proc/%d/cmdline 2>/dev/null; echo\n", pg, pg)
		}
	}
	cmds := map[int]string{}
	if b.Len() > 0 {
		lines, err := m.run(b.String())
		if err != nil {
			return err
		}
		for _, l := range lines {
			head, rest, _ := strings.Cut(l, " ")
			pg, _ := strconv.Atoi(head)
			cmds[pg] = strings.TrimSpace(rest)
		}
	}
	for sh, pg := range changed {
		sh.setForeground(cmds[pg])
	}
	return nil
}
