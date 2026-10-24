// sudoprobe は sudo モードの形を試す。PC アプリがパスワードを持ち、SSH の exec のチャネルで sudo に渡す。
// サーバーには何も置かない。確かめること
//   - パスワードが合っているかを、ON にするときに確かめられるか
//   - コマンドを sudo の権限で実行できるか
//   - sudo で起動した sftp-server と SFTP で話し、root のファイルを読み書きできるか
//   - パスワードがプロセスの一覧に出ないか
//   - NOPASSWD の設定があるときに、同じ手順が崩れないか
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var (
	addr = flag.String("addr", "127.0.0.1:2222", "SSH の接続先")
	user = flag.String("user", "asuta", "ユーザー名")
	pass = flag.String("pass", "drag", "SSH と sudo のパスワード")
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

// sudo のプロンプトを出さず、キャッシュした認証も使わずに、毎回標準入力の1行目をパスワードとして読ませる。
// キャッシュを使うと1行目を読まないので、パスワードがコマンドの入力に混ざる
const sudoPrefix = "sudo -k -S -p '' "

type result struct {
	out, errOut string
	code        int
	took        time.Duration
}

// run は cmd を exec のチャネルで実行する。stdin は先頭に書く
func run(c *ssh.Client, cmd, stdin string) result {
	s, err := c.NewSession()
	must(err)
	defer s.Close()
	var out, errOut bytes.Buffer
	s.Stdout, s.Stderr = &out, &errOut
	s.Stdin = strings.NewReader(stdin)
	t := time.Now()
	err = s.Run(cmd)
	r := result{out: out.String(), errOut: errOut.String(), took: time.Since(t)}
	var ee *ssh.ExitError
	if errors.As(err, &ee) {
		r.code = ee.ExitStatus()
	} else {
		must(err)
	}
	return r
}

const (
	okMark     = "DRAG-SUDO-OK"
	promptMark = "DRAG-PW:"
)

// sudoSFTP は sudo の権限で sftp-server を起動し、その標準入出力で SFTP を話す。
// sudo はパスワードを1行だけ読み、残りは sftp-server に渡る。
// パスワードが違うと sudo は次の行をパスワードとして読みにいくので、SFTP のデータを読ませないように、
// sudo が通ったことを1行出させてから SFTP を始める。プロンプトが2回出たらパスワードが違う
func sudoSFTP(c *ssh.Client, server, password string) (*sftp.Client, *ssh.Session, error) {
	s, err := c.NewSession()
	if err != nil {
		return nil, nil, err
	}
	in, _ := s.StdinPipe()
	out, _ := s.StdoutPipe()
	errPipe, _ := s.StderrPipe()
	inner := "sh -c 'echo " + okMark + "; exec " + server + "'"
	cmd := "sudo -n " + inner
	if password != "" {
		cmd = "sudo -k -S -p " + promptMark + " " + inner
	}
	if err := s.Start(cmd); err != nil {
		return nil, nil, err
	}
	// Start の前に書くと sshd に捨てられる
	if password != "" {
		io.WriteString(in, password+"\n")
	}

	res := make(chan error, 2)
	br := bufio.NewReader(out)
	go func() {
		l, err := br.ReadString('\n')
		if err == nil && l != okMark+"\n" {
			err = fmt.Errorf("思っていない出力 %q", l)
		}
		res <- err
	}()
	go func() {
		var all []byte
		b := make([]byte, 4096)
		for {
			n, err := errPipe.Read(b)
			all = append(all, b[:n]...)
			if bytes.Count(all, []byte(promptMark)) >= 2 {
				res <- fmt.Errorf("パスワードが違う %q", all)
				io.Copy(io.Discard, errPipe)
				return
			}
			if err != nil {
				if len(all) > 0 {
					res <- fmt.Errorf("%s", strings.TrimSpace(string(all)))
				}
				return
			}
		}
	}()
	select {
	case err = <-res:
	case <-time.After(10 * time.Second):
		err = errors.New("sudo から返事がない")
	}
	if err != nil {
		s.Close()
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
			s.Close()
			return nil, nil, m.err
		}
		return m.fc, s, nil
	case <-time.After(5 * time.Second):
		s.Close()
		return nil, nil, errors.New("SFTP の最初の往復が返らない")
	}
}

func uidOf(fi os.FileInfo) uint32 { return fi.Sys().(*sftp.FileStat).UID }

func writeFile(fc *sftp.Client, path, body string, flags int) error {
	f, err := fc.OpenFile(path, flags)
	if err != nil {
		return err
	}
	if _, err := f.Write([]byte(body)); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func readFile(fc *sftp.Client, path string) (string, error) {
	f, err := fc.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	return string(b), err
}

func main() {
	flag.Parse()
	c, err := ssh.Dial("tcp", *addr, &ssh.ClientConfig{
		User:            *user,
		Auth:            []ssh.AuthMethod{ssh.Password(*pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	must(err)
	defer c.Close()

	// sudo モードを ON にするとき
	r := run(c, "sudo -n true", "")
	check("パスワードなしでは通らない", r.code != 0, strings.TrimSpace(r.errOut))
	r = run(c, sudoPrefix+"-v", *pass+"\n")
	check("正しいパスワードで通る", r.code == 0, fmt.Sprintf("exit %d  %v", r.code, r.took.Round(time.Millisecond)))
	r = run(c, sudoPrefix+"-v", "wrong\n")
	check("違うパスワードで通らない", r.code != 0, fmt.Sprintf("exit %d  %v  %q", r.code, r.took.Round(time.Millisecond), strings.TrimSpace(r.errOut)))

	// コマンド
	r = run(c, sudoPrefix+"id -un", *pass+"\n")
	check("コマンドを root で実行する", strings.TrimSpace(r.out) == "root", fmt.Sprintf("%q  %v", strings.TrimSpace(r.out), r.took.Round(time.Millisecond)))

	// sftp-server の場所は sshd_config の Subsystem の行から取る
	r = run(c, `sed -n 's/^[[:space:]]*Subsystem[[:space:]]\{1,\}sftp[[:space:]]\{1,\}//p' /etc/ssh/sshd_config /etc/ssh/sshd_config.d/*.conf 2>/dev/null`, "")
	server := strings.Fields(r.out + " ")[0]
	check("sftp-server の場所", strings.HasPrefix(server, "/"), server)

	// 普段の SFTP では読めないこと
	plain, err := sftp.NewClient(c)
	must(err)
	_, err = plain.ReadDir("/root")
	check("普段の SFTP では /root を開けない", err != nil, fmt.Sprint(err))
	must(writeFile(plain, "/tmp/drag-owned-by-user.txt", "user\n", os.O_WRONLY|os.O_CREATE|os.O_TRUNC))
	plain.Close()

	t := time.Now()
	fc, s, err := sudoSFTP(c, server, *pass)
	must(err)
	check("sudo の SFTP を始める", true, time.Since(t).Round(time.Millisecond).String())

	ps := run(c, "ps -eo user,args | grep '[s]ftp-server'", "")
	check("パスワードがプロセスの一覧に出ない", !strings.Contains(ps.out, *pass) && strings.Contains(ps.out, "root"), strings.Join(strings.Fields(ps.out), " "))

	ents, err := fc.ReadDir("/root")
	check("/root を開ける", err == nil, fmt.Sprintf("%d 件 %v", len(ents), err))
	shadow, err := readFile(fc, "/etc/shadow")
	check("/etc/shadow を読める", err == nil && strings.HasPrefix(shadow, "root:"), fmt.Sprint(err))

	const conf = "/etc/drag-test.conf"
	err = writeFile(fc, conf, "a=1\n", os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	fi, _ := fc.Stat(conf)
	check("/etc にファイルを作る。持ち主は root", err == nil && fi != nil && uidOf(fi) == 0, fmt.Sprint(err))
	err = fc.Rename(conf, conf+".bak")
	check("名前を変える", err == nil, fmt.Sprint(err))
	err = fc.Chmod(conf+".bak", 0o600)
	fi, _ = fc.Stat(conf + ".bak")
	check("権限を変える", err == nil && fi != nil && fi.Mode().Perm() == 0o600, fmt.Sprint(fi.Mode().Perm()))
	err = fc.Remove(conf + ".bak")
	check("消す", err == nil, fmt.Sprint(err))

	// 利用者のファイルを sudo で書き換えても、持ち主は変わらない
	err = writeFile(fc, "/tmp/drag-owned-by-user.txt", "edited by root\n", os.O_WRONLY|os.O_TRUNC)
	fi, _ = fc.Stat("/tmp/drag-owned-by-user.txt")
	check("利用者のファイルを書き換えても持ち主はそのまま", err == nil && fi != nil && uidOf(fi) != 0, fmt.Sprintf("uid %d", uidOf(fi)))

	// サービスの操作。unit を SFTP で置き、systemctl を sudo で実行する
	const unit = "/etc/systemd/system/drag-test.service"
	must(writeFile(fc, unit, "[Service]\nExecStart=/bin/sleep infinity\n", os.O_WRONLY|os.O_CREATE|os.O_TRUNC))
	r = run(c, sudoPrefix+"sh -c 'systemctl daemon-reload && systemctl start drag-test && systemctl is-active drag-test'", *pass+"\n")
	check("サービスを起動する", strings.TrimSpace(r.out) == "active", fmt.Sprintf("%q %s %v", strings.TrimSpace(r.out), strings.TrimSpace(r.errOut), r.took.Round(time.Millisecond)))
	r = run(c, sudoPrefix+"systemctl stop drag-test", *pass+"\n")
	check("サービスを止める", r.code == 0, strings.TrimSpace(r.errOut))
	must(fc.Remove(unit))
	run(c, sudoPrefix+"systemctl daemon-reload", *pass+"\n")

	log := run(c, sudoPrefix+"journalctl -t sudo -n 30 --no-pager -o cat", *pass+"\n")
	logged := ""
	for _, l := range strings.Split(log.out, "\n") {
		if strings.Contains(l, "sftp-server") {
			logged = strings.TrimSpace(l)
		}
	}
	check("sudo のログに残る", logged != "", logged)
	fc.Close()
	s.Close()

	// 違うパスワードでは SFTP を始めない
	t = time.Now()
	_, _, err = sudoSFTP(c, server, "wrong")
	check("違うパスワードでは SFTP を始めない", err != nil, fmt.Sprintf("%v  %v", err, time.Since(t).Round(time.Millisecond)))
	time.Sleep(200 * time.Millisecond)
	r = run(c, "ps -eo args | grep '[s]udo -k'", "")
	check("止めた sudo が残らない", r.out == "", r.out)

	// NOPASSWD の設定があると sudo はパスワードを読まない。sudo -n で確かめて、パスワードを送らずに始める
	fc, s, err = sudoSFTP(c, server, *pass)
	must(err)
	must(writeFile(fc, "/etc/sudoers.d/drag-test", *user+" ALL=(ALL) NOPASSWD: ALL\n", os.O_WRONLY|os.O_CREATE|os.O_TRUNC))
	must(fc.Chmod("/etc/sudoers.d/drag-test", 0o440))
	fc.Close()
	s.Close()
	r = run(c, "sudo -n true", "")
	check("NOPASSWD のときは sudo -n が通る", r.code == 0, strings.TrimSpace(r.errOut))
	fc, s, err = sudoSFTP(c, server, *pass)
	if err == nil {
		_, err = fc.ReadDir("/root")
		fc.Close()
		s.Close()
	}
	check("NOPASSWD のときにパスワードを送ると SFTP が崩れる", err != nil, fmt.Sprint(err))
	fc, s, err = sudoSFTP(c, server, "")
	check("NOPASSWD のときはパスワードを送らずに始める", err == nil, fmt.Sprint(err))
	if err == nil {
		must(fc.Remove("/etc/sudoers.d/drag-test"))
		fc.Close()
		s.Close()
	}
	r = run(c, "sudo -n true", "")
	check("NOPASSWD の設定を戻した", r.code != 0, strings.TrimSpace(r.errOut))

	if failed {
		os.Exit(1)
	}
}
