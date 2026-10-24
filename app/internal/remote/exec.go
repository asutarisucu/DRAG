package remote

import (
	"errors"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// ExecResult は Exec で実行したコマンドの結果
type ExecResult struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Code      int    `json:"code"`      // 終了コード。時間切れで止めたときは -1
	TimedOut  bool   `json:"timedOut"`  // 時間切れで止めた
	Truncated bool   `json:"truncated"` // 出力が execLimit を超えたので、頭だけを返した
}

// execLimit は Exec で返す標準出力と標準エラーそれぞれの上限
const execLimit = 1 << 20

// limitBuf は、上限までを残し、超えた分は捨てる
type limitBuf struct {
	mu   sync.Mutex
	b    strings.Builder
	over bool
}

func (l *limitBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	keep := p
	if rest := execLimit - l.b.Len(); rest < len(p) {
		l.over = true
		keep = p[:max(rest, 0)]
	}
	l.b.Write(keep)
	// 捨てた分も書いたことにする。短く返すと、送り手が書き込みの失敗として止まる
	return len(p), nil
}

// Exec は、端末を使わずにコマンドを1つ実行し、出力と終了コードを返す。sudo モードなら sudo で実行する。
// コマンドは標準入力で渡すので、引用符をそのまま書ける。コマンドの標準入力は空になる
func (s *Server) Exec(command string, timeout time.Duration) (ExecResult, error) {
	cmd, stdin := s.command(`c=$(cat); exec /bin/sh -c "$c"`, command)
	sess, err := s.ssh.NewSession()
	if err != nil {
		return ExecResult{}, err
	}
	defer sess.Close()
	var out, errOut limitBuf
	sess.Stdin = strings.NewReader(stdin)
	sess.Stdout, sess.Stderr = &out, &errOut
	if err := sess.Start(cmd); err != nil {
		return ExecResult{}, err
	}
	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()
	var r ExecResult
	select {
	case err = <-done:
	case <-time.After(timeout):
		// チャネルを閉じるだけでは、sshd はコマンドが終わるまで待つ。先に KILL を送る
		r.TimedOut = true
		sess.Signal(ssh.SIGKILL)
		sess.Close()
		select {
		case err = <-done:
		case <-time.After(2 * time.Second):
			err = &ssh.ExitMissingError{}
		}
	}
	var exit *ssh.ExitError
	var missing *ssh.ExitMissingError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		r.Code = exit.ExitStatus()
	case r.TimedOut && errors.As(err, &missing):
	default:
		return ExecResult{}, err
	}
	if r.TimedOut {
		r.Code = -1
	}
	r.Stdout, r.Stderr, r.Truncated = out.b.String(), errOut.b.String(), out.over || errOut.over
	return r, nil
}
