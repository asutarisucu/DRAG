package remote

import (
	"strings"
	"testing"
	"time"
)

// 端末なしでコマンドを実行し、標準出力・標準エラー・終了コードを分けて返す。sudo モードなら sudo で動く
func TestExec(t *testing.T) {
	s := dialTrusted(t)
	r, err := s.Exec(`echo "it's $((6*7))"; echo oops >&2; exit 3`, 10*time.Second)
	if err != nil || r.Stdout != "it's 42\n" || r.Stderr != "oops\n" || r.Code != 3 || r.TimedOut {
		t.Fatalf("%+v %v", r, err)
	}
	r, err = s.Exec("sleep 5", 300*time.Millisecond)
	if err != nil || !r.TimedOut || r.Code != -1 {
		t.Fatalf("時間切れ %+v %v", r, err)
	}
	r, _ = s.Exec("head -c 1100000 /dev/zero", 10*time.Second)
	if !r.Truncated || len(r.Stdout) != execLimit {
		t.Fatalf("上限 %d %v", len(r.Stdout), r.Truncated)
	}
	if r, _ := s.Exec("id -u", 10*time.Second); strings.TrimSpace(r.Stdout) == "0" {
		t.Fatal("sudo モードでないのに root で動いた")
	}
	if err := s.EnableSudo("drag"); err != nil {
		t.Fatal(err)
	}
	defer s.DisableSudo()
	if r, err := s.Exec("id -u", 10*time.Second); err != nil || strings.TrimSpace(r.Stdout) != "0" {
		t.Fatalf("sudo モード %+v %v", r, err)
	}
}
