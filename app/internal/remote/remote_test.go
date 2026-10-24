package remote

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// 検証用のサーバー (dev/testserver) に対して試す。
// DRAG_TEST_SSH=127.0.0.1:2222 のように接続先を渡したときだけ動く
func testConfig(t *testing.T) Config {
	addr := os.Getenv("DRAG_TEST_SSH")
	if addr == "" {
		t.Skip("DRAG_TEST_SSH が無い")
	}
	host, port, _ := strings.Cut(addr, ":")
	p, err := strconv.Atoi(port)
	if err != nil {
		p = 22
	}
	return Config{Host: host, Port: p, User: "asuta", Password: "drag"}
}

func TestDialTrustsHostKeyOnlyWhenConfirmed(t *testing.T) {
	cfg := testConfig(t)
	kh := filepath.Join(t.TempDir(), "known_hosts")

	_, err := Dial(cfg, kh, "")
	var he *HostKeyError
	if !errors.As(err, &he) || he.Mismatch || !strings.HasPrefix(he.Fingerprint, "SHA256:") {
		t.Fatalf("未登録の鍵で HostKeyError にならない: %v", err)
	}
	if _, err := Dial(cfg, kh, "SHA256:違う"); !errors.As(err, &he) {
		t.Fatalf("違う指紋で受け入れた: %v", err)
	}
	s, err := Dial(cfg, kh, he.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	// 登録したあとは、指紋を渡さなくてもつながる
	s, err = Dial(cfg, kh, "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	b, _ := os.ReadFile(kh)
	if lines := strings.Count(string(b), "\n"); lines != 1 {
		t.Fatalf("known_hosts の行数 %d", lines)
	}
	// 同じ種類の別の鍵の行に置き換えると、なりすましとして断る
	pub, _, _ := ed25519.GenerateKey(nil)
	other, _ := ssh.NewPublicKey(pub)
	host := strings.Fields(string(b))[0]
	os.WriteFile(kh, []byte(knownhosts.Line([]string{host}, other)+"\n"), 0o600)
	if _, err := Dial(cfg, kh, he.Fingerprint); !errors.As(err, &he) || !he.Mismatch {
		t.Fatalf("鍵が違うのにつながった: %v", err)
	}
}

func dialTrusted(t *testing.T) *Server {
	cfg := testConfig(t)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	_, err := Dial(cfg, kh, "")
	var he *HostKeyError
	if !errors.As(err, &he) {
		t.Fatal(err)
	}
	s, err := Dial(cfg, kh, he.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestListDirPutsFoldersFirst(t *testing.T) {
	s := dialTrusted(t)
	home, err := s.Home()
	if err != nil || home != "/home/asuta" {
		t.Fatalf("ホーム %q %v", home, err)
	}
	ents, err := s.ListDir("/")
	if err != nil {
		t.Fatal(err)
	}
	seenFile := false
	for _, e := range ents {
		if !e.Dir {
			seenFile = true
		} else if seenFile {
			t.Fatalf("ファイルの後にフォルダ %s", e.Name)
		}
	}
	// Ubuntu の /bin は /usr/bin へのリンク
	for _, e := range ents {
		if e.Name == "bin" && (!e.Link || !e.Dir) {
			t.Fatalf("/bin がフォルダへのリンクにならない: %+v", e)
		}
	}
}

func TestProbeFindsTestServerFeatures(t *testing.T) {
	s := dialTrusted(t)
	info, err := s.Probe()
	if err != nil {
		t.Fatal(err)
	}
	// 検証用のサーバーは Ubuntu 24.04 で、systemd・screen・tmux と、偽の docker (dev/testserver/fakedocker.sh) がある
	want := Info{OS: "Linux", Arch: info.Arch, Distro: "Ubuntu 24.04", Systemd: true, Docker: true, Screen: true, Tmux: true}
	if info != want || info.Arch == "" {
		t.Fatalf("%+v", info)
	}
}

// testDir は検証用のサーバーの /tmp に、テストごとの空のフォルダを作る
func testDir(t *testing.T, s *Server) string {
	d := "/tmp/drag-test-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if err := s.sftp.Mkdir(d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.sftp.RemoveAll(d) })
	return d
}

func TestFileOps(t *testing.T) {
	s := dialTrusted(t)
	d := testDir(t, s)

	// 引用符と空白を含む名前でも、作る・複製する・名前を変えるができる
	odd := `it's a "file".txt`
	if err := s.CreateFile(d + "/" + odd); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateFile(d + "/" + odd); !errors.Is(err, ErrExists) {
		t.Fatalf("同じ名前で作れた: %v", err)
	}
	if n := s.UniqueName(d, odd, false); n != `it's a "file" (2).txt` {
		t.Fatalf("番号を足した名前 %q", n)
	}
	if n := s.UniqueName(d, odd, true); n != `it's a "file" - コピー.txt` {
		t.Fatalf("複製の名前 %q", n)
	}
	if err := s.Mkdir(d + "/sub dir"); err != nil {
		t.Fatal(err)
	}
	if err := s.Copy(d+"/"+odd, d+"/sub dir/"+odd); err != nil {
		t.Fatal(err)
	}
	if err := s.Copy(d+"/sub dir", d+"/sub dir 2"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.sftp.Stat(d + "/sub dir 2/" + odd); err != nil {
		t.Fatalf("フォルダの中まで複製されない: %v", err)
	}
	if err := s.Rename(d+"/sub dir 2", d+"/sub dir"); !errors.Is(err, ErrExists) {
		t.Fatalf("同じ名前へ変えられた: %v", err)
	}

	// ゴミ箱へ移して戻す
	item, err := s.Trash(d + "/" + odd)
	if err != nil {
		t.Fatal(err)
	}
	if s.exists(d + "/" + odd) {
		t.Fatal("元の場所に残っている")
	}
	f, _ := s.sftp.Open("/home/asuta/.local/share/Trash/info/" + item.Name + ".trashinfo")
	b := make([]byte, 512)
	n, _ := f.Read(b)
	f.Close()
	if !strings.Contains(string(b[:n]), "Path="+d+"/it%27s%20a%20%22file%22.txt") {
		t.Fatalf("trashinfo %q", b[:n])
	}
	to, err := s.Restore(item)
	if err != nil || to != d+"/"+odd {
		t.Fatalf("戻した先 %q %v", to, err)
	}
	if s.exists("/home/asuta/.local/share/Trash/info/" + item.Name + ".trashinfo") {
		t.Fatal("trashinfo が残っている")
	}
}

func TestUploadDownloadAndStat(t *testing.T) {
	s := dialTrusted(t)
	d := testDir(t, s)

	// PC の側にフォルダを作って送る
	local := filepath.Join(t.TempDir(), "up dir")
	os.MkdirAll(filepath.Join(local, "sub"), 0o755)
	os.WriteFile(filepath.Join(local, "a.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(local, "sub", "日本語.txt"), bytes.Repeat([]byte("x"), 300000), 0o644)
	var last, total int64
	p, err := s.Upload(local, d, true, func(done, tot int64) { last, total = done, tot })
	if err != nil || p != d+"/up dir" {
		t.Fatalf("%q %v", p, err)
	}
	if last != 300005 || total != 300005 {
		t.Fatalf("進み %d / %d", last, total)
	}
	// 同じ名前でもう一度。keep なら番号を足し、上書きなら同じ場所に混ぜる
	if p, _ := s.Upload(local, d, true, func(int64, int64) {}); p != d+"/up dir (2)" {
		t.Fatalf("keep %q", p)
	}
	os.WriteFile(filepath.Join(local, "b.txt"), []byte("new"), 0o644)
	if p, _ := s.Upload(local, d, false, func(int64, int64) {}); p != d+"/up dir" {
		t.Fatalf("上書き %q", p)
	}
	ents, _ := s.ListDir(d + "/up dir")
	if len(ents) != 3 {
		t.Fatalf("混ぜたあと %+v", ents)
	}

	// フォルダを zip にして受け取る
	zipPath := filepath.Join(t.TempDir(), "out.zip")
	if err := s.Download(d+"/up dir", zipPath, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	zr.Close()
	slices.Sort(names)
	want := []string{"up dir/", "up dir/a.txt", "up dir/b.txt", "up dir/sub/", "up dir/sub/日本語.txt"}
	if !slices.Equal(names, want) {
		t.Fatalf("zip の中 %q", names)
	}

	// ファイルはそのまま受け取る
	filePath := filepath.Join(t.TempDir(), "a.txt")
	if err := s.Download(d+"/up dir/a.txt", filePath, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filePath); string(b) != "hello" {
		t.Fatalf("中身 %q", b)
	}

	pr, err := s.Stat(d + "/up dir/a.txt")
	if err != nil || pr.Owner != "asuta" || pr.Size != 5 || !strings.HasPrefix(pr.Mode, "-rw") {
		t.Fatalf("%+v %v", pr, err)
	}
}

func TestForegroundAndReplay(t *testing.T) {
	s := dialTrusted(t)
	fg := make(chan string, 10)
	var last int64
	sh, err := s.OpenShell(80, 24, "", ShellEvents{
		Output:     func(_ []byte, end int64) { last = end },
		Foreground: func(c string) { fg <- c },
		Exit:       func() {},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	wait := func() string {
		select {
		case c := <-fg:
			return c
		case <-time.After(3 * time.Second):
			return "(来ない)"
		}
	}
	time.Sleep(400 * time.Millisecond)
	sh.Write([]byte("echo before\r"))
	time.Sleep(400 * time.Millisecond)
	sh.Write([]byte("sleep 1\r"))
	if c := wait(); c != "sleep 1" {
		t.Fatalf("前面 %q", c)
	}
	if sh.Foreground() != "sleep 1" {
		t.Fatalf("Foreground %q", sh.Foreground())
	}
	// 打った行から後ろだけを渡し直す。前に打った echo は入らない
	data, end := sh.Replay(true)
	if !bytes.Contains(data, []byte("sleep 1")) || bytes.Contains(data, []byte("before")) || end != last {
		t.Fatalf("渡し直す出力 %q %d %d", data, end, last)
	}
	if all, _ := sh.Replay(false); !bytes.Contains(all, []byte("before")) {
		t.Fatalf("全部 %q", all)
	}
	if c := wait(); c != "" {
		t.Fatalf("終わっても前面が戻らない %q", c)
	}
	if cwd, err := sh.Cwd(); cwd != "/home/asuta" || err != nil {
		t.Fatalf("cwd %q %v", cwd, err)
	}
}

func TestTextFileAndLog(t *testing.T) {
	s := dialTrusted(t)
	d := testDir(t, s)
	write := func(p string, b []byte) {
		f, err := s.sftp.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		f.Write(b)
		f.Close()
	}

	write(d+"/a.txt", []byte("こんにちは\n"))
	write(d+"/bin", []byte{1, 0, 2})
	write(d+"/sjis", []byte{0x82, 0xa0})
	tf, err := s.ReadText(d+"/a.txt", 1024)
	if err != nil || tf.Text != "こんにちは\n" {
		t.Fatalf("%+v %v", tf, err)
	}
	if _, err := s.ReadText(d+"/bin", 1024); !errors.Is(err, ErrBinary) {
		t.Fatalf("バイナリ %v", err)
	}
	if _, err := s.ReadText(d+"/sjis", 1024); !errors.Is(err, ErrNotUTF8) {
		t.Fatalf("UTF-8 でない %v", err)
	}
	if _, err := s.ReadText(d+"/a.txt", 4); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("大きい %v", err)
	}
	// 開いたあとに書き換えられていたら、force なしでは書かない
	time.Sleep(1100 * time.Millisecond)
	write(d+"/a.txt", []byte("changed\n"))
	if _, err := s.WriteText(d+"/a.txt", "mine\n", tf.ModTime, false); !errors.Is(err, ErrChanged) {
		t.Fatalf("変わったのに書いた %v", err)
	}
	if _, err := s.WriteText(d+"/a.txt", "mine\n", 0, true); err != nil {
		t.Fatal(err)
	}
	if tf, _ := s.ReadText(d+"/a.txt", 1024); tf.Text != "mine\n" {
		t.Fatalf("%q", tf.Text)
	}

	// ログは末尾から行の頭で区切って読み、前を読み足せる
	var lines []byte
	for i := 1; i <= 100; i++ {
		lines = fmt.Appendf(lines, "line %03d\n", i)
	}
	write(d+"/app.log", lines)
	c, err := s.ReadTail(d+"/app.log", -1, 100)
	if err != nil || !strings.HasPrefix(c.Text, "line 0") || !strings.HasSuffix(c.Text, "line 100\n") || c.End != int64(len(lines)) {
		t.Fatalf("末尾 %+v %v", c, err)
	}
	prev, _ := s.ReadTail(d+"/app.log", c.Start, 100)
	if prev.End != c.Start || !strings.HasSuffix(prev.Text, "\n") || !strings.HasPrefix(prev.Text, "line ") {
		t.Fatalf("前 %+v", prev)
	}

	// 追記を受け取る
	got := make(chan string, 10)
	stop, err := s.Follow(d+"/app.log", c.End, func(b []byte) { got <- string(b) }, func(error) {})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	time.Sleep(500 * time.Millisecond)
	sess, _ := s.ssh.NewSession()
	sess.Run("echo 'line 101' >> " + d + "/app.log")
	sess.Close()
	select {
	case g := <-got:
		if g != "line 101\n" {
			t.Fatalf("追記 %q", g)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("追記が届かない")
	}
}

func TestSessionsListsScreenAndTmux(t *testing.T) {
	s := dialTrusted(t)
	run := func(cmd string) {
		sess, _ := s.ssh.NewSession()
		defer sess.Close()
		sess.Run(cmd)
	}
	run("screen -dmS drag-t1; tmux new -d -s drag-t2")
	t.Cleanup(func() { run("screen -S drag-t1 -X quit; tmux kill-session -t drag-t2") })
	list, err := s.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	var scr, tm *Session
	for i := range list {
		switch {
		case list[i].Kind == "screen" && strings.HasSuffix(list[i].Name, ".drag-t1"):
			scr = &list[i]
		case list[i].Kind == "tmux" && list[i].Name == "drag-t2":
			tm = &list[i]
		}
	}
	if scr == nil || tm == nil || scr.Attached || tm.Attached || tm.Desc != "1" || scr.Desc == "" {
		t.Fatalf("%+v", list)
	}
}

func TestOpenShellKnowsPIDAndStartsInCwd(t *testing.T) {
	s := dialTrusted(t)
	var mu sync.Mutex
	var out strings.Builder
	exited := make(chan struct{})
	sh, err := s.OpenShell(80, 24, "/var/log", ShellEvents{
		Output: func(p []byte, _ int64) {
			mu.Lock()
			out.Write(p)
			mu.Unlock()
		},
		Foreground: func(string) {},
		Exit:       func() { close(exited) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if sh.PID <= 0 {
		t.Fatalf("PID %d", sh.PID)
	}
	sh.Write([]byte("pwd; exit\r"))
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		t.Fatal("シェルが終わらない")
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(out.String(), "/var/log\r\n") || strings.Contains(out.String(), "DRAG-") {
		t.Fatalf("出力 %q", out.String())
	}
}

// 画面からの入力は並行して届く。重なって書いても接続が切れず、どの入力も届くこと
func TestShellWriteFromManyGoroutines(t *testing.T) {
	s := dialTrusted(t)
	var mu sync.Mutex
	var out strings.Builder
	exited := make(chan struct{})
	sh, err := s.OpenShell(80, 24, "", ShellEvents{
		Output: func(p []byte, _ int64) {
			mu.Lock()
			out.Write(p)
			mu.Unlock()
		},
		Foreground: func(string) {},
		Exit:       func() { close(exited) },
	})
	if err != nil {
		t.Fatal(err)
	}
	sh.Write([]byte("cat > /dev/null\r"))
	var wg sync.WaitGroup
	// 長さを変えておくと、重なったときにパケットの長さの欄と中身が食い違う
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				sh.Write([]byte(strings.Repeat("x", i+j+1)))
			}
		}()
	}
	wg.Wait()
	sh.Write([]byte("\r\x04echo DONE-$((6*7)); exit\r"))
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		t.Fatal("シェルが終わらない")
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(out.String(), "DONE-42") {
		t.Fatalf("出力の末尾 %q", out.String()[max(0, out.Len()-300):])
	}
}

// sudo モードの間は、ホームの外と root のファイルを扱え、OFF に戻すと扱えなくなること
func TestSudoMode(t *testing.T) {
	s := dialTrusted(t)
	if _, err := s.ListDir("/root"); err == nil {
		t.Fatal("OFF なのに /root を開けた")
	}
	start := time.Now()
	if err := s.EnableSudo("wrong"); !errors.Is(err, ErrSudoPassword) {
		t.Fatalf("違うパスワード: %v", err)
	}
	t.Logf("違うパスワードと分かるまで %v", time.Since(start).Round(time.Millisecond))
	if s.Sudo() {
		t.Fatal("違うパスワードで ON になった")
	}
	if err := s.EnableSudo("drag"); err != nil {
		t.Fatal(err)
	}
	if !s.Sudo() {
		t.Fatal("ON にならない")
	}
	if _, err := s.ListDir("/root"); err != nil {
		t.Fatalf("/root を開けない: %v", err)
	}
	// sudo モードは端末ごと。同じ接続でも、ほかの端末からは OFF に見える
	if phone := s.As("phone"); phone.Sudo() {
		t.Fatal("ほかの端末でも ON になった")
	} else if _, err := phone.ListDir("/root"); err == nil {
		t.Fatal("ほかの端末から /root を開けた")
	}
	etc, err := s.ListDir("/etc")
	if err != nil {
		t.Fatal(err)
	}
	// 鍵は sudo モードと関係なく、ログインしたユーザーで書けるかで付ける
	if i := slices.IndexFunc(etc, func(e Entry) bool { return e.Name == "hostname" }); i < 0 || !etc[i].Locked {
		t.Fatalf("/etc/hostname に書けないと出ない")
	}

	p := "/etc/drag-sudo-test"
	t.Cleanup(func() {
		s.EnableSudo("drag")
		s.fs().Remove(p)
		s.fs().Remove(p + "-copy")
	})
	if err := s.CreateFile(p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteText(p, "a=1\n", 0, true); err != nil {
		t.Fatal(err)
	}
	// stat と cp は sudo を付けたコマンドで動く。パスワードの行がコマンドの入力に混ざらないこと
	pr, err := s.Stat(p)
	if err != nil || pr.Owner != "root" {
		t.Fatalf("%+v %v", pr, err)
	}
	if err := s.Copy(p, p+"-copy"); err != nil {
		t.Fatal(err)
	}
	if tf, err := s.ReadText(p+"-copy", 1<<20); err != nil || tf.Text != "a=1\n" {
		t.Fatalf("%q %v", tf.Text, err)
	}
	item, err := s.Trash(p + "-copy")
	if err != nil {
		t.Fatal(err)
	}
	home, _ := s.Home()
	info, err := s.sftp.Stat(home + "/.local/share/Trash/info/" + item.Name + ".trashinfo")
	if err != nil || info.Sys().(*sftp.FileStat).UID == 0 {
		t.Fatalf("ゴミ箱の info が利用者の持ち物でない: %v", err)
	}
	if _, err := s.Restore(item); err != nil {
		t.Fatal(err)
	}

	s.DisableSudo()
	if s.Sudo() {
		t.Fatal("OFF にならない")
	}
	if _, err := s.ListDir("/root"); err == nil {
		t.Fatal("OFF に戻したのに /root を開けた")
	}
	if _, err := s.WriteText(p, "b=2\n", 0, true); err == nil {
		t.Fatal("OFF に戻したのに /etc に書けた")
	}
}

// NOPASSWD の設定があるサーバーでは、パスワードを送らずに ON にし、コマンドも sudo -n で動かす
func TestSudoModeNopasswd(t *testing.T) {
	s := dialTrusted(t)
	if err := s.EnableSudo("drag"); err != nil {
		t.Fatal(err)
	}
	const rule = "/etc/sudoers.d/drag-test"
	f, err := s.fs().OpenFile(rule, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("asuta ALL=(ALL) NOPASSWD: ALL\n"))
	f.Close()
	s.fs().Chmod(rule, 0o440)
	t.Cleanup(func() {
		s.EnableSudo("")
		s.fs().Remove(rule)
	})
	s.DisableSudo()

	if err := s.EnableSudo(""); err != nil {
		t.Fatalf("パスワードなしで ON にならない: %v", err)
	}
	if pw, _ := s.SudoPassword(); pw != "" {
		t.Fatal("NOPASSWD なのにパスワードを持っている")
	}
	if pr, err := s.Stat("/root"); err != nil || pr.Owner != "root" {
		t.Fatalf("%+v %v", pr, err)
	}
	if _, err := s.ListDir("/root"); err != nil {
		t.Fatal(err)
	}
}
