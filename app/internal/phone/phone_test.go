package phone

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
)

// start は 127.0.0.1 の空いているポートで待ち受け、証明書を確かめないクライアントを返す
func start(t *testing.T, call Caller) (*Server, string, *http.Client) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	s := New(Options{
		Dir:    t.TempDir(),
		Assets: fstest.MapFS{"index.html": {Data: []byte("<title>app</title>")}},
		Call:   call,
	})
	if err := s.Start(addr); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{
		Jar:       jar,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	return s, "https://" + addr, hc
}

func get(t *testing.T, hc *http.Client, u string) (int, string) {
	t.Helper()
	res, err := hc.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestPairingLetsOnlyRegisteredDevicesIn(t *testing.T) {
	s, base, hc := start(t, func(Device, string, []json.RawMessage) (any, error) { return nil, nil })

	if code, body := get(t, hc, base+"/"); code != http.StatusForbidden || !strings.Contains(body, "not registered") {
		t.Fatalf("登録前に画面が出た %d %q", code, body)
	}
	if code, _ := get(t, hc, base+"/pair?t=wrong"); code != http.StatusGone {
		t.Fatalf("違うトークン %d", code)
	}
	u, exp, err := s.NewPairURL()
	if err != nil || time.Until(exp) > PairTTL || !strings.HasPrefix(u, base+"/pair?t=") {
		t.Fatalf("%q %v %v", u, exp, err)
	}
	if code, body := get(t, hc, u); code != http.StatusOK || body != "<title>app</title>" {
		t.Fatalf("登録して画面へ移らない %d %q", code, body)
	}
	// 同じ QR コードは2度使えない
	other, _ := cookiejar.New(nil)
	hc2 := &http.Client{Jar: other, Transport: hc.Transport}
	if code, _ := get(t, hc2, u); code != http.StatusGone {
		t.Fatalf("使用済みのトークンで登録できた %d", code)
	}
	devs := s.Devices()
	if len(devs) != 1 || devs[0].Hash != "" {
		t.Fatalf("%+v", devs)
	}
	kinds := []string{}
	for _, l := range s.History() {
		kinds = append(kinds, l.Kind)
	}
	if strings.Join(kinds, ",") != "pair-failed,pair,pair-failed,denied" {
		t.Fatalf("履歴 %v", kinds)
	}

	if err := s.Revoke(devs[0].ID); err != nil {
		t.Fatal(err)
	}
	if code, _ := get(t, hc, base+"/"); code != http.StatusForbidden {
		t.Fatalf("無効にした端末で画面が出た %d", code)
	}
}

func TestWebSocketCallsAndEvents(t *testing.T) {
	s, base, hc := start(t, func(dev Device, m string, args []json.RawMessage) (any, error) {
		var n int
		json.Unmarshal(args[0], &n)
		return map[string]any{"m": m, "n": n * 2, "dev": dev.Name}, nil
	})
	u, _, _ := s.NewPairURL()
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 15) AppleWebKit/537.36 Chrome/141.0 Mobile Safari/537.36")
	res, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "wss" + strings.TrimPrefix(base, "https") + "/api/ws"
	// 別のサイトからの接続は断る
	if _, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPClient: hc, HTTPHeader: http.Header{"Origin": {"https://evil.example"}}}); err == nil {
		t.Fatal("別のサイトからつながった")
	}
	ws, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPClient: hc, HTTPHeader: http.Header{"Origin": {base}}})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	ws.Write(ctx, websocket.MessageText, []byte(`{"id":7,"m":"Twice","a":[21]}`))
	_, b, err := ws.Read(ctx)
	if err != nil || string(b) != `{"id":7,"r":{"dev":"Android (Chrome)","m":"Twice","n":42}}` {
		t.Fatalf("%s %v", b, err)
	}
	s.Broadcast("shell:out", "sh1", "aGk=", 2)
	_, b, err = ws.Read(ctx)
	if err != nil || string(b) != `{"ev":"shell:out","a":["sh1","aGk=",2]}` {
		t.Fatalf("%s %v", b, err)
	}
	// 無効にすると、つないでいる画面も切る
	s.Revoke(s.Devices()[0].ID)
	if _, _, err := ws.Read(ctx); err == nil {
		t.Fatal("無効にした端末の接続が残った")
	}
}

func TestDeviceName(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (Linux; Android 15; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36":                   "Android (Chrome)",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1": "iPhone (Safari)",
		"curl/8.0": "ブラウザ",
	} {
		if got := deviceName(ua); got != want {
			t.Errorf("%q → %q", ua, got)
		}
	}
}

// QR コードを続けて作り直しても、出したものはどれも1回ずつ使える
func TestPairTokensStayValidUntilUsed(t *testing.T) {
	s, _, hc := start(t, func(Device, string, []json.RawMessage) (any, error) { return nil, nil })
	u1, _, _ := s.NewPairURL()
	u2, _, _ := s.NewPairURL()
	for _, u := range []string{u2, u1} {
		jar, _ := cookiejar.New(nil)
		c := &http.Client{Jar: jar, Transport: hc.Transport}
		if code, _ := get(t, c, u); code != http.StatusOK {
			t.Fatalf("%d", code)
		}
	}
	if len(s.Devices()) != 2 {
		t.Fatalf("%+v", s.Devices())
	}
	for i := 0; i < pairLimit+2; i++ {
		s.NewPairURL()
	}
	if len(s.pairs) != pairLimit {
		t.Fatalf("トークンが %d 個残った", len(s.pairs))
	}
}
