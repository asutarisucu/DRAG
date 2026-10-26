package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// 合言葉を知らない相手には、/call を送らない
func TestCallChecksServerFirst(t *testing.T) {
	var calls atomic.Int32
	serve := func(token string) *httptest.Server {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Drag-Proof", helloProof(token, r.Header.Get("X-Drag-Nonce")))
			w.WriteHeader(http.StatusNoContent)
		})
		mux.HandleFunc("POST /call", func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Write([]byte(`{"r":"ok"}`))
		})
		return httptest.NewServer(mux)
	}
	other := serve("other")
	defer other.Close()
	c := &client{addr: strings.TrimPrefix(other.URL, "http://"), token: "secret"}
	if err := c.call(nil, "EnableSudo", "s1", "password"); err == nil || !strings.Contains(err.Error(), "DRAG ではありません") {
		t.Fatalf("%v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("合言葉を知らない相手に送った")
	}
	real := serve("secret")
	defer real.Close()
	c.addr = strings.TrimPrefix(real.URL, "http://")
	var out string
	if err := c.call(&out, "Servers"); err != nil || out != "ok" {
		t.Fatalf("%q %v", out, err)
	}
}
