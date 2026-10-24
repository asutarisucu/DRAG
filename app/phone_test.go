package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/asutarisucu/DRAG/app/internal/phone"
)

// スマホからの呼び出しは、決めたメソッドだけを、引数の数と型を確かめてから呼ぶ
func TestPhoneCall(t *testing.T) {
	a := NewApp()
	dev := phone.Device{ID: "d1"}
	raw := func(vs ...string) []json.RawMessage {
		out := []json.RawMessage{}
		for _, v := range vs {
			out = append(out, json.RawMessage(v))
		}
		return out
	}

	res, err := a.phoneCall(dev, "Servers", raw())
	if err != nil {
		t.Fatal(err)
	}
	if list, ok := res.([]ServerInfo); !ok || len(list) != 0 {
		t.Fatalf("%#v", res)
	}
	// 戻り値のないメソッドは nil を返す
	if res, err := a.phoneCall(dev, "ShellResize", raw(`"t1"`, `80`, `24`)); res != nil || err != nil {
		t.Fatalf("%#v %v", res, err)
	}
	if _, err := a.phoneCall(dev, "ListDir", raw(`"s1"`, `"/"`)); err == nil || !strings.Contains(err.Error(), "つながっていません") {
		t.Fatalf("つないでいないサーバー: %v", err)
	}
	for _, c := range []struct {
		m    string
		args []json.RawMessage
		want string
	}{
		{"PickUploadFiles", raw(), "スマホからは使えません"},
		{"NewPairing", raw(), "スマホからは使えません"},
		{"ListDir", raw(`"s1"`), "引数の数"},
		{"ShellResize", raw(`"t1"`, `"wide"`, `24`), "引数を読めません"},
	} {
		if _, err := a.phoneCall(dev, c.m, c.args); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.m, err)
		}
	}
}

// スマホで打ったキーを行に組み立てる。消すキーとエスケープの並びは文字に入れない
func TestTypedLines(t *testing.T) {
	key := "d1/t9"
	if got := typedLines(key, "ls -l"); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	// 1文字消して打ち直し、矢印 (ESC [ A と ESC O B) を挟んで Enter
	if got := typedLines(key, "\x7fa\x1b[A\x1bOB /tmp\r"); strings.Join(got, "|") != "入力 ls -a /tmp" {
		t.Fatalf("%q", got)
	}
	// Ctrl+C で打っていた行は捨てる。複数行をまとめて送っても1行ずつ残す
	if got := typedLines(key, "rm -rf x\x03echo 1\recho 2\r"); strings.Join(got, "|") != "入力 echo 1|入力 echo 2" {
		t.Fatalf("%q", got)
	}
}

// 記録にはパスワードを残さない
func TestAuditHidesPasswords(t *testing.T) {
	dev := phone.Device{ID: "d1"}
	got := audit(dev, "Connect", []json.RawMessage{json.RawMessage(`{"user":"asuta","host":"example","password":"secret"}`)})
	if len(got) != 1 || strings.Contains(got[0], "secret") || got[0] != "接続 asuta@example" {
		t.Fatalf("%q", got)
	}
	got = audit(dev, "EnableSudo", []json.RawMessage{json.RawMessage(`"s1"`), json.RawMessage(`"secret"`)})
	if len(got) != 1 || strings.Contains(got[0], "secret") {
		t.Fatalf("%q", got)
	}
	if got := audit(dev, "ListDir", []json.RawMessage{json.RawMessage(`"s1"`), json.RawMessage(`"/"`)}); got != nil {
		t.Fatalf("読むだけの呼び出しを残した %q", got)
	}
}

// 証明書を取れるドメインの形
func TestDomainRe(t *testing.T) {
	for d, want := range map[string]bool{
		"drag.example.com": true, "example.jp": true, "a-b.example.co.jp": true, "xn--eckwd4c7c.xn--zckzah": true,
		"example": false, "-a.example.com": false, "a..example.com": false, "a_b.example.com": false, "example.c": false, "192.0.2.1": false,
	} {
		if domainRe.MatchString(d) != want {
			t.Errorf("%s: %v", d, !want)
		}
	}
}
