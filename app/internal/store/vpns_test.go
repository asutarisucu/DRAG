package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestVPNsAreSavedProtected(t *testing.T) {
	dir := t.TempDir()
	s := NewVPNs(dir)
	key := "PrivateKey = yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk="
	v1, err := s.Add(VPN{Name: "office", Type: "wg", Config: "[Interface]\n" + key + "\n"})
	if err != nil {
		t.Fatal(err)
	}
	v2, _ := s.Add(VPN{Name: "lab", Type: "wg", Config: "[Interface]\n"})
	if v1.ID == v2.ID {
		t.Fatalf("ID がぶつかった %s", v1.ID)
	}
	// 鍵はファイルにそのまま書かない
	b, err := os.ReadFile(filepath.Join(dir, "vpns.dat"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("yAnz5TF")) || bytes.Contains(b, []byte("office")) {
		t.Fatal("鍵か名前がそのまま書かれている")
	}
	got, err := NewVPNs(dir).Get(v1.ID)
	if err != nil || got.Name != "office" || got.Config != "[Interface]\n"+key+"\n" {
		t.Fatalf("%+v %v", got, err)
	}
	if err := s.Remove(v1.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List()
	if len(list) != 1 || list[0].ID != v2.ID {
		t.Fatalf("%+v", list)
	}
	if _, err := s.Get(v1.ID); err == nil {
		t.Fatal("消した VPN が読めた")
	}
}
