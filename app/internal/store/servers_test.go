package store

import "testing"

func TestPutUpdatesSameHostPortUser(t *testing.T) {
	s := NewServers(t.TempDir())
	id1, err := s.Put(Server{Name: "web", Host: "198.51.100.24", Port: 22, User: "asuta", Auth: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	id2, _ := s.Put(Server{Name: "pi", Host: "192.168.1.20", Port: 22, User: "pi", Auth: "pass"})
	id3, _ := s.Put(Server{Name: "web01", Host: "198.51.100.24", Port: 22, User: "asuta", Auth: "key", KeyFile: "C:/k"})
	if id1 != id3 || id1 == id2 {
		t.Fatalf("ID %s %s %s", id1, id2, id3)
	}
	list, _ := s.List()
	if len(list) != 2 || list[0].Name != "web01" || list[0].Auth != "key" {
		t.Fatalf("%+v", list)
	}
	if err := s.Remove(id2); err != nil {
		t.Fatal(err)
	}
	list, _ = s.List()
	if len(list) != 1 {
		t.Fatalf("%+v", list)
	}
	// 消したあとに足しても、残っている ID とぶつからない
	id4, _ := s.Put(Server{Name: "new", Host: "192.168.1.1", Port: 22, User: "admin"})
	if id4 == id1 {
		t.Fatalf("ID がぶつかった %s", id4)
	}
}
