package remote

import (
	"strings"
	"testing"
	"time"
)

// ほかのユーザーの screen は、sudo モードのときだけ持ち主の名前を付けて並べる
func TestSessionsListsOtherUsersScreen(t *testing.T) {
	s := dialTrusted(t)
	if err := s.EnableSudo("drag"); err != nil {
		t.Fatal(err)
	}
	if r, err := s.Exec("sudo -u zuser screen -dmS drag-other", 10*time.Second); err != nil || r.Code != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	t.Cleanup(func() {
		s.EnableSudo("drag")
		s.Exec("sudo -u zuser screen -S drag-other -X quit", 10*time.Second)
		s.DisableSudo()
	})
	find := func(list []Session) *Session {
		for i := range list {
			if list[i].User == "zuser" && strings.HasSuffix(list[i].Name, ".drag-other") {
				return &list[i]
			}
		}
		return nil
	}
	list, err := s.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	got := find(list)
	if got == nil || got.Kind != "screen" || got.Attached || time.Since(time.Unix(got.LastUsed, 0)) > time.Minute {
		t.Fatalf("%+v", list)
	}
	for _, x := range list {
		if x.User == "asuta" {
			t.Fatalf("自分のセッションをほかのユーザーのものとして出した %+v", x)
		}
	}
	// sudo モードを OFF にすると見えない
	s.DisableSudo()
	if list, _ := s.Sessions(); find(list) != nil {
		t.Fatalf("OFF でも見えた %+v", list)
	}
}
