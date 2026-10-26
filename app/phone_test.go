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
	if got := typedLines(key, "\x7fa\x1b[A\x1bOB /tmp\r"); strings.Join(got, "|") != "コマンド ls -a /tmp" {
		t.Fatalf("%q", got)
	}
	// Ctrl+C で打っていた行は捨てる。複数行をまとめて送っても1行ずつ残す
	if got := typedLines(key, "rm -rf x\x03echo 1\recho 2\r"); strings.Join(got, "|") != "コマンド echo 1|コマンド echo 2" {
		t.Fatalf("%q", got)
	}
}

// 記録にはパスワードを残さない
func TestAuditHidesPasswords(t *testing.T) {
	a := NewApp()
	dev := phone.Device{ID: "d1"}
	got := a.audit(dev, "Connect", []json.RawMessage{json.RawMessage(`{"user":"asuta","host":"example","password":"secret"}`)})
	if len(got) != 1 || strings.Contains(got[0], "secret") || got[0] != "接続 asuta@example" {
		t.Fatalf("%q", got)
	}
	got = a.audit(dev, "EnableSudo", []json.RawMessage{json.RawMessage(`"s1"`), json.RawMessage(`"secret"`)})
	if len(got) != 1 || strings.Contains(got[0], "secret") {
		t.Fatalf("%q", got)
	}
	if got := a.audit(dev, "ListDir", []json.RawMessage{json.RawMessage(`"s1"`), json.RawMessage(`"/"`)}); got != nil {
		t.Fatalf("読むだけの呼び出しを残した %q", got)
	}
	// 前面にいるのがシェルだと確かめられない行は残さない
	if got := a.audit(dev, "ShellInput", []json.RawMessage{json.RawMessage(`"t1"`), json.RawMessage(`"secret\r"`)}); got != nil {
		t.Fatalf("確かめていない行を残した %q", got)
	}
}

// 引数の足りない呼び出しでも、記録を作るところで落ちずに、呼ぶ前の確かめで断る
func TestPhoneCallWithoutArgs(t *testing.T) {
	a := NewApp()
	for _, m := range []string{"Connect", "AddVPN", "RestoreEntry", "TrashDelete", "ShellInput"} {
		if _, err := a.phoneCall(phone.Device{ID: "d1"}, m, nil); err == nil || !strings.Contains(err.Error(), "引数の数") {
			t.Errorf("%s: %v", m, err)
		}
	}
}

// スマホからは、PC のほかのパスや UNC のパスを鍵として開かせない
func TestPhoneConnectKeyFile(t *testing.T) {
	a := NewApp()
	for _, kf := range []string{`\attacker.example\s\k`, `\?\UNC\attacker\s\k`, `id_ed25519`, `C:\drag-test-not-saved\id_ed25519`} {
		req, _ := json.Marshal(ConnectRequest{Host: "127.0.0.1", Port: 1, User: "x", Auth: "key", KeyFile: kf})
		res, err := a.phoneCall(phone.Device{ID: "d1"}, "Connect", []json.RawMessage{req})
		if r, ok := res.(ConnectResult); err != nil || !ok || !strings.Contains(r.Error, "鍵") {
			t.Errorf("%s: %#v %v", kf, res, err)
		}
	}
}

// エージェントの頼みを PC で確かめない設定は、スマホからもエージェントからも切り替えられない
func TestAgentAskOnlyFromPC(t *testing.T) {
	a := NewApp()
	if _, err := a.phoneCall(phone.Device{ID: "d1"}, "SetAgentAsk", []json.RawMessage{json.RawMessage(`false`)}); err == nil || !strings.Contains(err.Error(), "スマホからは使えません") {
		t.Fatalf("スマホ: %v", err)
	}
	if _, err := a.agentCall("SetAgentAsk", []json.RawMessage{json.RawMessage(`false`)}); err == nil || !strings.Contains(err.Error(), "エージェントからは使えません") {
		t.Fatalf("エージェント: %v", err)
	}
	if _, err := (&App{core: a.core, dev: agentDev}).SetAgentAsk(false); err == nil {
		t.Fatal("PC の画面のほかから切り替えた")
	}
}
