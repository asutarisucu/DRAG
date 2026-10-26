package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// エージェントの窓口は、合言葉が合うときだけ、決めたメソッドを呼ぶ
func TestAgentHandler(t *testing.T) {
	a := NewApp()
	srv := httptest.NewServer(a.agentHandler("secret"))
	defer srv.Close()
	call := func(token, body string) (int, map[string]any) {
		req, _ := http.NewRequest("POST", srv.URL+"/call", strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	// /hello は合言葉そのものを返さず、合言葉を知っていることだけを示す
	req, _ := http.NewRequest("GET", srv.URL+"/hello", nil)
	req.Header.Set("X-Drag-Nonce", "0123456789abcdef")
	if res, err := http.DefaultClient.Do(req); err != nil || res.Header.Get("X-Drag-Proof") != helloProof("secret", "0123456789abcdef") {
		t.Fatalf("hello %v", err)
	}
	if code, _ := call("", `{"m":"Servers","a":[]}`); code != http.StatusForbidden {
		t.Fatalf("合言葉なし %d", code)
	}
	if code, _ := call("wrong", `{"m":"Servers","a":[]}`); code != http.StatusForbidden {
		t.Fatalf("違う合言葉 %d", code)
	}
	if code, out := call("secret", `{"m":"Servers","a":[]}`); code != http.StatusOK || out["e"] != nil {
		t.Fatalf("%d %v", code, out)
	} else if list, ok := out["r"].([]any); !ok || len(list) != 0 {
		t.Fatalf("%v", out)
	}
	for body, want := range map[string]string{
		`{"m":"PickUploadFiles","a":[]}`: "エージェントからは使えません",
		`{"m":"NewPairing","a":["lan"]}`: "エージェントからは使えません",
		`{"m":"Exec","a":["s1","ls"]}`:   "引数の数",
		`{"m":"Exec","a":["s1","ls",5]}`: "つながっていません",
		`{"m":"AgentPair","a":["wan"]}`:  `"public" か "lan"`,
	} {
		_, out := call("secret", body)
		if e, _ := out["e"].(string); !strings.Contains(e, want) {
			t.Errorf("%s: %s", body, e)
		}
	}
}
