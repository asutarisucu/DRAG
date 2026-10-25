package tsvpn

import (
	"bufio"
	"context"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 試験用の Headscale (dev/headscale) に対して、Docker の中の tester で動かす。
// DRAG_TEST_TS_CONTROL にログインサーバー、DRAG_TEST_TS_DIR に authkey と apikey のあるフォルダを渡す。
// tailnet の中の peer:22 は検証用の SSH サーバーへつながっている
func testEnv(t *testing.T) (control, dir string) {
	control, dir = os.Getenv("DRAG_TEST_TS_CONTROL"), os.Getenv("DRAG_TEST_TS_DIR")
	if control == "" || dir == "" {
		t.Skip("DRAG_TEST_TS_CONTROL と DRAG_TEST_TS_DIR が無い")
	}
	return control, dir
}

func read(t *testing.T, dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

// sshBanner は tailnet の中の peer:22 へつなぎ、SSH の最初の行を読む
func sshBanner(t *testing.T, n *Node) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := n.DialContext(ctx, "tcp", "peer:22")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "SSH-2.0-") {
		t.Fatalf("%q %v", line, err)
	}
}

func TestAuthKey(t *testing.T) {
	control, dir := testEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	n, err := open(ctx, Saved{Control: control}, read(t, dir, "authkey"))
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	t.Logf("tailnet %q、名前 %q、アドレス %v", n.Tailnet(), n.Name(), n.Addr())
	if !n.Addr().IsValid() {
		t.Fatal("tailnet のアドレスが無い")
	}
	sshBanner(t, n)
}

// ブラウザのログインの代わりに Headscale の API で端末を登録し、保存した状態でつなぎ直せることを確かめる
func TestLoginAndReopen(t *testing.T) {
	control, dir := testEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	n, u, err := Login(ctx, control)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ログインのページ %s", u)
	if !strings.Contains(u, "/register/") {
		n.Close()
		t.Fatalf("ログインのページではない: %q", u)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, control+"/api/v1/node/register?"+url.Values{"user": {"drag"}, "key": {path.Base(u)}}.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+read(t, dir, "apikey"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		n.Close()
		t.Fatalf("Headscale に登録できない: %v %v", resp, err)
	}
	resp.Body.Close()
	if err := n.Wait(ctx); err != nil {
		n.Close()
		t.Fatal(err)
	}
	if n.Tailnet() == "" || n.Name() == "" {
		n.Close()
		t.Fatalf("tailnet %q、名前 %q", n.Tailnet(), n.Name())
	}
	saved := n.Saved()
	n.Close()

	s, err := Parse(saved)
	if err != nil {
		t.Fatal(err)
	}
	n, err = Open(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	sshBanner(t, n)
}
