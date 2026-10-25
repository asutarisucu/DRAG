// Package store は、PC に置く DRAG の記録を読み書きする
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
)

// Server は保存済みのサーバー。パスワードは保存しない
type Server struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	Auth    string `json:"auth"` // "pass" か "key"
	KeyFile string `json:"keyFile"`
	VPN     string `json:"vpn,omitempty"` // 通す VPN の番号 (vpns.dat)。VPN なしなら空
}

// Servers は servers.json に保存済みのサーバーを持つ
type Servers struct {
	path string
	mu   sync.Mutex
}

func NewServers(dir string) *Servers {
	return &Servers{path: filepath.Join(dir, "servers.json")}
}

func (s *Servers) load() ([]Server, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return []Server{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Server
	return list, json.Unmarshal(b, &list)
}

func (s *Servers) save(list []Server) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	// 書きかけで落ちても前のファイルが残るように、別名で書いてから置き換える
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Servers) List() ([]Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

// Put は、同じホスト・ポート・ユーザーのものがあれば上書きし、なければ足す。保存した ID を返す
func (s *Servers) Put(v Server) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load()
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(list, func(o Server) bool { return o.Host == v.Host && o.Port == v.Port && o.User == v.User })
	if i >= 0 {
		v.ID = list[i].ID
		list[i] = v
	} else {
		n := 1
		for _, o := range list {
			if id, err := strconv.Atoi(o.ID); err == nil && id >= n {
				n = id + 1
			}
		}
		v.ID = strconv.Itoa(n)
		list = append(list, v)
	}
	return v.ID, s.save(list)
}

func (s *Servers) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load()
	if err != nil {
		return err
	}
	return s.save(slices.DeleteFunc(list, func(o Server) bool { return o.ID == id }))
}
