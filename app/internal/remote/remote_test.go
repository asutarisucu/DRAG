package remote

import (
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
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

func TestOpenShellKnowsPIDAndStartsInCwd(t *testing.T) {
	s := dialTrusted(t)
	var mu sync.Mutex
	var out strings.Builder
	exited := make(chan struct{})
	sh, err := s.OpenShell(80, 24, "/var/log", func(p []byte) {
		mu.Lock()
		out.Write(p)
		mu.Unlock()
	}, func() { close(exited) })
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
