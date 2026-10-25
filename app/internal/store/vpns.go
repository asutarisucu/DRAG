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

// VPN は保存した VPN。設定ファイルの中に鍵があり、パスワードも持つので、
// ファイルごと DPAPI で守って vpns.dat に置く
type VPN struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`             // "wg" WireGuard、"ovpn" OpenVPN、"ikev2" IKEv2、"sstp" SSTP、"ac" AnyConnect 互換、"l2tp" L2TP/IPsec
	Config   string `json:"config,omitempty"` // WireGuard の .conf、OpenVPN の .ovpn の中身
	Server   string `json:"server,omitempty"` // 設定ファイルを使わない種類のつなぎ先
	CA       string `json:"ca,omitempty"`     // サーバーの証明書を確かめる CA (PEM)。空なら Windows が信頼する CA
	Group    string `json:"group,omitempty"`  // AnyConnect 互換でログインするときに選ぶグループ。空ならサーバーの既定
	PSK      string `json:"psk,omitempty"`    // L2TP/IPsec の事前共有キー
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}

type VPNs struct {
	path string
	mu   sync.Mutex
}

func NewVPNs(dir string) *VPNs {
	return &VPNs{path: filepath.Join(dir, "vpns.dat")}
}

func (s *VPNs) load() ([]VPN, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return []VPN{}, nil
	}
	if err != nil {
		return nil, err
	}
	if b, err = unprotect(b); err != nil {
		return nil, err
	}
	var list []VPN
	return list, json.Unmarshal(b, &list)
}

func (s *VPNs) save(list []VPN) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	if b, err = protect(b); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *VPNs) List() ([]VPN, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *VPNs) Get(id string) (VPN, error) {
	list, err := s.List()
	if err != nil {
		return VPN{}, err
	}
	i := slices.IndexFunc(list, func(o VPN) bool { return o.ID == id })
	if i < 0 {
		return VPN{}, errors.New("保存した VPN にありません")
	}
	return list[i], nil
}

// Add は新しい VPN を足し、番号を付けて返す
func (s *VPNs) Add(v VPN) (VPN, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load()
	if err != nil {
		return VPN{}, err
	}
	n := 1
	for _, o := range list {
		if id, err := strconv.Atoi(o.ID); err == nil && id >= n {
			n = id + 1
		}
	}
	v.ID = strconv.Itoa(n)
	return v, s.save(append(list, v))
}

func (s *VPNs) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load()
	if err != nil {
		return err
	}
	return s.save(slices.DeleteFunc(list, func(o VPN) bool { return o.ID == id }))
}
