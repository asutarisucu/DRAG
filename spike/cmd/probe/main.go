// probe は PC アプリの代わりに、検証用サーバーへエージェントを入れて試す。
// 確かめること
//   - エージェントの転送から起動までにかかる時間
//   - エージェントを入れ替えても、落としても、シェルが切れずに残るか
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var (
	addr = flag.String("addr", "127.0.0.1:2222", "SSH の接続先")
	user = flag.String("user", "asuta", "ユーザー名")
	pass = flag.String("pass", "drag", "パスワード。sudo にも使う")
)

type probe struct {
	sc   *ssh.Client
	ftp  *sftp.Client
	home string
	bins map[string]string // 版 → ローカルの実行ファイル
}

var failed bool

func check(name string, ok bool, detail string) {
	mark := "OK  "
	if !ok {
		mark = "NG  "
		failed = true
	}
	fmt.Printf("%s %s  %s\n", mark, name, detail)
}

func main() {
	flag.Parse()
	p := &probe{bins: map[string]string{}}
	must(p.build("v1", "v2"))

	t := time.Now()
	sc, err := ssh.Dial("tcp", *addr, &ssh.ClientConfig{
		User:            *user,
		Auth:            []ssh.AuthMethod{ssh.Password(*pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	must(err)
	p.sc = sc
	p.ftp, err = sftp.NewClient(sc, sftp.UseConcurrentWrites(true))
	must(err)
	p.home = strings.TrimSpace(p.run("echo $HOME"))
	fmt.Printf("SSH 接続 %v  home=%s arch=%s\n", time.Since(t).Round(time.Millisecond), p.home, strings.TrimSpace(p.run("uname -m")))

	p.sudo("systemctl disable --now drag-agent 2>/dev/null; rm -f /etc/systemd/system/drag-agent.service; systemctl daemon-reload")
	p.run("pkill -f '[.]drag/bin/drag' ; rm -rf ~/.drag")

	// 1. 初回の配置とサービスの登録
	t = time.Now()
	up := p.upload("v1")
	p.installService("process")
	ready := p.waitVersion("v1")
	check("初回の配置から起動まで", ready > 0, fmt.Sprintf("計 %v (転送 %v)", time.Since(t).Round(time.Millisecond), up))

	// 2. セッションを作り、シェルに状態を持たせる
	id := strings.TrimSpace(p.post("/sessions"))
	ws := p.attach(id)
	ws.Write(context.Background(), websocket.MessageText, []byte(`{"cols":120,"rows":30}`))
	ws.Write(context.Background(), websocket.MessageBinary, []byte("X=hello; cd /tmp; echo pid=$$\r"))
	m := expect(ws, `pid=(\d+)`)
	pid := ""
	if m != nil {
		pid = m[1]
	}
	check("セッションを作る", pid != "", "id="+id+" shell pid="+pid)
	// ロケールが無いと、bash は打った日本語を \346\227\245 のように表示する
	ws.Write(context.Background(), websocket.MessageBinary, []byte("echo 日本語 $LANG\r"))
	m = expect(ws, `echo 日本語 \$LANG\r\n[^日]*日本語 (\S+)\r`)
	ws.CloseNow()
	lang := ""
	if m != nil {
		lang = m[1]
	}
	check("サービスから起動したシェルで日本語を打てる", m != nil, "LANG="+lang)
	same := func(name string) {
		ws := p.attach(id)
		if ws == nil {
			check(name, false, "つなげない")
			return
		}
		defer ws.CloseNow()
		replay := expect(ws, `pid=`+pid)
		ws.Write(context.Background(), websocket.MessageBinary, []byte("echo \"[$X][$PWD][$$]\"\r"))
		got := expect(ws, `\[hello\]\[/tmp\]\[`+pid+`\]`)
		check(name, replay != nil && got != nil, fmt.Sprintf("出力の再送 %v, 変数と cwd と pid %v", replay != nil, got != nil))
	}

	// 3. 版を上げてエージェントを入れ替える
	t = time.Now()
	up = p.upload("v2")
	p.post("/restart")
	ready = p.waitVersion("v2")
	check("入れ替えにかかる時間", ready > 0, fmt.Sprintf("計 %v (転送 %v)", time.Since(t).Round(time.Millisecond), up))
	same("入れ替えた後もシェルが残る")

	// 4. エージェントを強制終了する。systemd が起動し直す
	p.run("kill -9 $(systemctl show -p MainPID --value drag-agent)")
	time.Sleep(300 * time.Millisecond)
	p.waitVersion("v2")
	same("強制終了の後もシェルが残る")

	// 5. 対照。KillMode を既定に戻すと、再起動で保持役も止められるはず
	p.installService("control-group")
	p.sudo("systemctl restart drag-agent")
	p.waitVersion("v2")
	gone := !strings.Contains(p.get("/sessions"), id)
	check("対照 KillMode=control-group では消える", gone, "")

	// 6. sudo なしのサーバー。サービスにせず、SSH から起動する
	p.sudo("systemctl disable --now drag-agent; rm -f /etc/systemd/system/drag-agent.service; systemctl daemon-reload")
	p.run("pkill -f '[.]drag/bin/drag'; rm -rf ~/.drag/run")
	p.startPlain()
	p.waitVersion("v2")
	id = strings.TrimSpace(p.post("/sessions"))
	ws = p.attach(id)
	ws.Write(context.Background(), websocket.MessageBinary, []byte("X=hello; cd /tmp; echo pid=$$\r"))
	if m := expect(ws, `pid=(\d+)`); m != nil {
		pid = m[1]
	}
	ws.CloseNow()
	p.run("pkill -9 -f '[.]drag/bin/drag agent'")
	time.Sleep(300 * time.Millisecond)
	p.startPlain()
	p.waitVersion("v2")
	same("サービスなしでもシェルが残る")

	p.run("pkill -f '[.]drag/bin/drag'")
	if failed {
		os.Exit(1)
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (p *probe) build(versions ...string) error {
	dir, err := os.MkdirTemp("", "drag-probe")
	if err != nil {
		return err
	}
	for _, v := range versions {
		out := filepath.Join(dir, "drag-"+v)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X main.version="+v, "-o", out, "./cmd/drag")
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		p.bins[v] = out
	}
	st, _ := os.Stat(p.bins[versions[0]])
	fmt.Printf("実行ファイル %.1f MB\n", float64(st.Size())/1e6)
	return nil
}

func (p *probe) run(cmd string) string {
	s, err := p.sc.NewSession()
	must(err)
	defer s.Close()
	out, _ := s.CombinedOutput(cmd)
	return string(out)
}

func (p *probe) sudo(cmd string) string {
	s, err := p.sc.NewSession()
	must(err)
	defer s.Close()
	s.Stdin = strings.NewReader(*pass + "\n")
	out, _ := s.CombinedOutput("sudo -S -p '' sh -c '" + cmd + "'")
	return string(out)
}

// 別名で送ってから名前を変える。動いている実行ファイルを書き換えずに済む
func (p *probe) upload(v string) time.Duration {
	t := time.Now()
	dst := p.home + "/.drag/bin/drag"
	must(p.ftp.MkdirAll(p.home + "/.drag/bin"))
	src, err := os.Open(p.bins[v])
	must(err)
	defer src.Close()
	f, err := p.ftp.Create(dst + ".new")
	must(err)
	_, err = f.ReadFrom(src)
	must(err)
	must(f.Close())
	must(p.ftp.Chmod(dst+".new", 0o755))
	must(p.ftp.PosixRename(dst+".new", dst))
	return time.Since(t).Round(time.Millisecond)
}

func (p *probe) installService(killMode string) {
	unit := fmt.Sprintf(`[Unit]
Description=DRAG agent

[Service]
User=%s
ExecStart=%s/.drag/bin/drag agent
Restart=always
KillMode=%s

[Install]
WantedBy=multi-user.target
`, *user, p.home, killMode)
	f, err := p.ftp.Create(p.home + "/.drag/drag-agent.service")
	must(err)
	f.Write([]byte(unit))
	f.Close()
	out := p.sudo("install -m 644 " + p.home + "/.drag/drag-agent.service /etc/systemd/system/ && systemctl daemon-reload && systemctl enable --now drag-agent")
	if strings.Contains(out, "rror") {
		fmt.Print(out)
	}
}

func (p *probe) startPlain() {
	p.run("setsid -f ~/.drag/bin/drag agent </dev/null >/dev/null 2>&1")
}

func (p *probe) client() *http.Client {
	sock := p.home + "/.drag/agent.sock"
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return p.sc.DialContext(ctx, "unix", sock)
		},
	}, Timeout: 5 * time.Second}
}

func (p *probe) get(path string) string {
	res, err := p.client().Get("http://drag" + path)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return string(b)
}

func (p *probe) post(path string) string {
	res, err := p.client().Post("http://drag"+path, "", nil)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return string(b)
}

// 指定の版が応答するまで待ち、かかった時間を返す。10 秒で諦めて 0 を返す
func (p *probe) waitVersion(v string) time.Duration {
	t := time.Now()
	for time.Since(t) < 10*time.Second {
		if p.get("/version") == v {
			return time.Since(t)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return 0
}

func (p *probe) attach(id string) *websocket.Conn {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws://drag/sessions/"+id, &websocket.DialOptions{HTTPClient: p.client()})
	if err != nil {
		return nil
	}
	ws.SetReadLimit(-1)
	return ws
}

// 出力を読み続け、pattern に合った時点で返す。3 秒で諦める
func expect(ws *websocket.Conn, pattern string) []string {
	re := regexp.MustCompile(pattern)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var buf bytes.Buffer
	for {
		if m := re.FindStringSubmatch(buf.String()); m != nil {
			return m
		}
		_, b, err := ws.Read(ctx)
		if err != nil {
			fmt.Printf("     出力 %q\n", tail(buf.String(), 300))
			return nil
		}
		buf.Write(b)
	}
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
