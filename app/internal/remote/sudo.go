package remote

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// sudo モード。ON の間は、PC アプリがパスワードをメモリに持ち、ファイルの操作は sudo で起動した
// sftp-server と話し、コマンドは sudo を付けて実行する。サーバーには何も置かない

// ErrSudoPassword は sudo のパスワードが違うとき
var ErrSudoPassword = errors.New("sudo のパスワードが違います")

const (
	sudoOK     = "DRAG-SUDO-OK"
	sudoPrompt = "DRAG-PW:"
)

type sudoMode struct {
	password string // NOPASSWD のサーバーでは空
	fc       *sftp.Client
	sess     *ssh.Session
}

func (m *sudoMode) close() {
	m.fc.Close()
	m.sess.Close()
}

// sftp-server の場所。sshd_config の Subsystem の行にあればそれを、internal-sftp なら決まった場所を探す
const sftpServerScript = `for c in $(sed -n "s/^[[:space:]]*Subsystem[[:space:]][[:space:]]*sftp[[:space:]][[:space:]]*\([^[:space:]]*\).*/\1/p" /etc/ssh/sshd_config /etc/ssh/sshd_config.d/*.conf 2>/dev/null) /usr/lib/openssh/sftp-server /usr/libexec/openssh/sftp-server /usr/lib/ssh/sftp-server /usr/libexec/sftp-server; do
[ -x "$c" ] && { echo "$c"; exit 0; }
done
exit 1`

// EnableSudo は sudo モードを ON にする。パスワードが違えば ErrSudoPassword を返す
func (s *Server) EnableSudo(password string) error {
	// NOPASSWD の設定があると sudo はパスワードを読まず、送ったパスワードの行がコマンドの入力に混ざる
	if s.runPlain("sudo -n true") == nil {
		password = ""
	} else if password == "" {
		return ErrSudoPassword
	}
	out, err := s.outputPlain("exec /bin/sh -c '" + sftpServerScript + "'")
	if err != nil {
		return errors.New("サーバーに sftp-server が見つかりません")
	}
	fc, sess, err := s.sudoSFTP(strings.TrimSpace(out), password)
	if err != nil {
		return err
	}
	s.mu.Lock()
	old := s.sudo[s.dev]
	s.sudo[s.dev] = &sudoMode{password: password, fc: fc, sess: sess}
	s.mu.Unlock()
	if old != nil {
		old.close()
	}
	return nil
}

// DisableSudo は sudo モードを OFF にし、持っていたパスワードを捨てる
func (s *Server) DisableSudo() {
	s.mu.Lock()
	m := s.sudo[s.dev]
	delete(s.sudo, s.dev)
	s.mu.Unlock()
	if m != nil {
		m.close()
	}
}

// Sudo は sudo モードが ON か
func (s *Server) Sudo() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sudo[s.dev] != nil
}

// SudoPassword は ON にしたときのパスワード。つなぎ直したあとに ON に戻すのに使う
func (s *Server) SudoPassword() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.sudo[s.dev]
	if m == nil {
		return "", false
	}
	return m.password, true
}

// SudoPasswords は、sudo モードが ON の端末ごとのパスワード。つなぎ直したあとに ON に戻すのに使う
func (s *Server) SudoPasswords() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for dev, m := range s.sudo {
		out[dev] = m.password
	}
	return out
}

// fs はファイルの操作に使う SFTP。sudo モードなら sudo で起動した sftp-server
func (s *Server) fs() *sftp.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.sudo[s.dev]; m != nil {
		return m.fc
	}
	return s.sftp
}

// command は /bin/sh で実行するスクリプトを、sudo モードなら sudo を付けて組み立てる。
// 返す stdin の頭には、sudo に読ませるパスワードの行が入る。sudo はその1行だけを読み、残りはスクリプトに渡る
func (s *Server) command(script, stdin string) (string, string) {
	s.mu.Lock()
	m := s.sudo[s.dev]
	s.mu.Unlock()
	switch {
	case m == nil:
		return "exec /bin/sh -c '" + script + "'", stdin
	case m.password == "":
		return "exec sudo -n /bin/sh -c '" + script + "'", stdin
	default:
		// -k で前の認証を使わせない。使われると sudo がパスワードの行を読まず、スクリプトに混ざる
		return "exec sudo -k -S -p '' /bin/sh -c '" + script + "'", m.password + "\n" + stdin
	}
}

func (s *Server) runPlain(cmd string) error {
	_, err := s.outputPlain(cmd)
	return err
}

func (s *Server) outputPlain(cmd string) (string, error) {
	sess, err := s.ssh.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	out, err := sess.Output(cmd)
	return string(out), err
}

// sudoSFTP は sudo の権限で sftp-server を起動し、その標準入出力で SFTP を話す。
// パスワードが違うと sudo は次の行をパスワードとして読みにいくので、SFTP のデータを読ませないように、
// sudo が通ったことを1行出させてから SFTP を始める。プロンプトが2回出たらパスワードが違う
func (s *Server) sudoSFTP(server, password string) (*sftp.Client, *ssh.Session, error) {
	sess, err := s.ssh.NewSession()
	if err != nil {
		return nil, nil, err
	}
	in, _ := sess.StdinPipe()
	out, _ := sess.StdoutPipe()
	errPipe, _ := sess.StderrPipe()
	inner := "sh -c 'echo " + sudoOK + "; exec " + server + "'"
	cmd := "exec sudo -n " + inner
	if password != "" {
		cmd = "exec sudo -k -S -p " + sudoPrompt + " " + inner
	}
	if err := sess.Start(cmd); err != nil {
		sess.Close()
		return nil, nil, err
	}
	// Start より前に書くと sshd に捨てられる
	if password != "" {
		io.WriteString(in, password+"\n")
	}

	res := make(chan error, 2)
	errDone := make(chan struct{})
	br := bufio.NewReader(out)
	go func() {
		l, err := br.ReadString('\n')
		switch {
		case err != nil:
			// sudo が先に終わった。理由は標準エラーのほうが返す
			<-errDone
			res <- errors.New("sudo が終わりました")
		case l != sudoOK+"\n":
			res <- fmt.Errorf("sudo から思っていない返事 %q", l)
		default:
			res <- nil
		}
	}()
	go func() {
		defer close(errDone)
		var all []byte
		b := make([]byte, 4096)
		for {
			n, err := errPipe.Read(b)
			all = append(all, b[:n]...)
			if bytes.Count(all, []byte(sudoPrompt)) >= 2 {
				res <- ErrSudoPassword
				io.Copy(io.Discard, errPipe)
				return
			}
			if err != nil {
				if msg := strings.TrimSpace(strings.ReplaceAll(string(all), sudoPrompt, "")); msg != "" {
					res <- errors.New(msg)
				}
				return
			}
		}
	}()
	select {
	case err = <-res:
	case <-time.After(15 * time.Second):
		err = errors.New("sudo から返事がありません")
	}
	if err != nil {
		sess.Close()
		return nil, nil, err
	}

	// SFTP の最初の往復が返らないときに止まらないようにする
	type made struct {
		fc  *sftp.Client
		err error
	}
	ch := make(chan made, 1)
	go func() {
		fc, err := sftp.NewClientPipe(br, in)
		ch <- made{fc, err}
	}()
	select {
	case m := <-ch:
		if m.err != nil {
			sess.Close()
			return nil, nil, m.err
		}
		return m.fc, sess, nil
	case <-time.After(5 * time.Second):
		sess.Close()
		return nil, nil, errors.New("sudo の SFTP が始まりません")
	}
}
