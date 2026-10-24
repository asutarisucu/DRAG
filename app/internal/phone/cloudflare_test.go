package phone

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// Cloudflare の API (developers.cloudflare.com/api の zones の list と dns/records の create・delete) の形で応える
// 偽のサーバーで、サブドメインからゾーンを上へたどり、TXT レコードを置いて消すことを確かめる
func TestCloudflareSetTXT(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	var created map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"Authentication error"}],"messages":[],"result":null}`))
			return
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/zones" && r.URL.Query().Get("name") == "example.com":
			w.Write([]byte(`{"success":true,"errors":[],"messages":[],"result":[{"id":"z1","name":"example.com","status":"active"}]}`))
		case r.Method == "GET" && r.URL.Path == "/zones":
			w.Write([]byte(`{"success":true,"errors":[],"messages":[],"result":[]}`))
		case r.Method == "POST" && r.URL.Path == "/zones/z1/dns_records" && r.Header.Get("Content-Type") == "application/json":
			json.NewDecoder(r.Body).Decode(&created)
			w.Write([]byte(`{"success":true,"errors":[],"messages":[],"result":{"id":"r1","zone_id":"z1","name":"_acme-challenge.drag.example.com","type":"TXT"}}`))
		case r.Method == "DELETE" && r.URL.Path == "/zones/z1/dns_records/r1":
			w.Write([]byte(`{"result":{"id":"r1"},"success":true,"errors":[],"messages":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	c := &Cloudflare{Token: "tok", API: srv.URL}
	remove, err := c.SetTXT(ctx, "_acme-challenge.drag.example.com", "v1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /zones?name=drag.example.com", "GET /zones?name=example.com", "POST /zones/z1/dns_records"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("%q", calls)
	}
	if !reflect.DeepEqual(created, map[string]any{"type": "TXT", "name": "_acme-challenge.drag.example.com", "content": "v1", "ttl": float64(60)}) {
		t.Fatalf("%v", created)
	}
	remove()
	if calls[len(calls)-1] != "DELETE /zones/z1/dns_records/r1" {
		t.Fatalf("%q", calls)
	}

	// トークンが違えば Cloudflare の文を出す
	bad := &Cloudflare{Token: "x", API: srv.URL}
	if _, err := bad.SetTXT(ctx, "_acme-challenge.drag.example.com", "v1"); err == nil || !strings.Contains(err.Error(), "Authentication error (10000)") {
		t.Fatalf("%v", err)
	}
	// ゾーンが見つからなければ、要る権限を書く
	if _, err := c.SetTXT(ctx, "_acme-challenge.drag.example.org", "v1"); err == nil || !strings.Contains(err.Error(), "drag.example.org のゾーンが見つかりません") {
		t.Fatalf("%v", err)
	}
}
