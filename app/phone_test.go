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
