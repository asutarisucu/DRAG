package remote

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	// 検証用のサーバーは Ubuntu 24.04 で、systemd・screen・tmux があり、docker は無い
	want := Info{OS: "Linux", Arch: info.Arch, Distro: "Ubuntu 24.04", Systemd: true, Screen: true, Tmux: true}
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
