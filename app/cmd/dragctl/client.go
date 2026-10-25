package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// client は、動いている DRAG のエージェントの窓口を呼ぶ。窓口のポートと合言葉は、設定フォルダの agent.json にある
type client struct {
	url, token string
}

// open は agent.json を読む。dev なら開発用の DRAG (wails dev) の設定フォルダを見る
func open(dev bool) (*client, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	name := "DRAG"
	if dev {
		name = "DRAG-dev"
	}
	b, err := os.ReadFile(filepath.Join(dir, name, "agent.json"))
	if err != nil {
		return nil, errors.New("DRAG が動いていません。PC で DRAG を起動してください")
	}
	var info struct {
		Port  int    `json:"port"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(b, &info); err != nil || info.Port == 0 {
		return nil, errors.New("agent.json を読めません。DRAG を起動し直してください")
	}
	return &client{url: "http://127.0.0.1:" + strconv.Itoa(info.Port) + "/call", token: info.Token}, nil
}

// call は DRAG のメソッドを呼び、戻り値を out に読む
func (c *client) call(out any, method string, args ...any) error {
	if args == nil {
		args = []any{}
	}
	body, err := json.Marshal(map[string]any{"m": method, "a": args})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.New("DRAG につながりません。PC で DRAG が動いているか確かめてください")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("DRAG が断りました (%s)。DRAG を起動し直すと agent.json が新しくなります", res.Status)
	}
	var r struct {
		R json.RawMessage `json:"r"`
		E string          `json:"e"`
	}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return err
	}
	if r.E != "" {
		return errors.New(r.E)
	}
	if out != nil {
		return json.Unmarshal(r.R, out)
	}
	return nil
}

// serverInfo は DRAG がつないでいるサーバー1台
type serverInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	User string `json:"user"`
	Host string `json:"host"`
	Home string `json:"home"`
}

// savedServer は DRAG に保存してあるサーバー1台
type savedServer struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	Auth    string `json:"auth"`
	KeyFile string `json:"keyFile"`
	VPN     string `json:"vpn"`
}

// server は、ID (s1 など)・名前・user@host・ホストのどれかから、つないでいるサーバーの ID を探す
func (c *client) server(ref string) (string, error) {
	var list []serverInfo
	if err := c.call(&list, "Servers"); err != nil {
		return "", err
	}
	for _, match := range []func(serverInfo) bool{
		func(s serverInfo) bool { return s.ID == ref },
		func(s serverInfo) bool { return s.Name == ref },
		func(s serverInfo) bool { return s.User+"@"+s.Host == ref },
		func(s serverInfo) bool { return s.Host == ref },
	} {
		for _, s := range list {
			if match(s) {
				return s.ID, nil
			}
		}
	}
	names := []string{}
	for _, s := range list {
		names = append(names, s.ID+" ("+s.Name+")")
	}
	if len(names) == 0 {
		return "", fmt.Errorf("%s につながっていません。いまつないでいるサーバーはありません", ref)
	}
	return "", fmt.Errorf("%s につながっていません。つないでいるのは %s です", ref, strings.Join(names, ", "))
}

// connect は保存済みのサーバーに、名前かホストでつなぐ。初めてのサーバーなら、鍵の指紋を返して止まる
func (c *client) connect(name, password, trust string) (any, error) {
	var saved []savedServer
	if err := c.call(&saved, "SavedServers"); err != nil {
		return nil, err
	}
	var hit *savedServer
	for i, s := range saved {
		if s.Name == name || s.Host == name || s.User+"@"+s.Host == name {
			hit = &saved[i]
			break
		}
	}
	if hit == nil {
		return nil, fmt.Errorf("保存済みのサーバーに %s がありません。DRAG の画面で一度つなぐと保存されます", name)
	}
	if hit.Auth == "pass" && password == "" {
		return nil, fmt.Errorf("%s はパスワードでつなぐサーバーです。パスワードを渡してください", hit.Name)
	}
	var r struct {
		ID      string `json:"id"`
		Home    string `json:"home"`
		Error   string `json:"error"`
		HostKey *struct {
			Fingerprint string `json:"fingerprint"`
			KeyType     string `json:"keyType"`
		} `json:"hostKey"`
	}
	req := map[string]any{"name": hit.Name, "host": hit.Host, "port": hit.Port, "user": hit.User, "auth": hit.Auth,
		"password": password, "keyFile": hit.KeyFile, "vpn": hit.VPN, "trust": trust}
	if err := c.call(&r, "Connect", req); err != nil {
		return nil, err
	}
	if r.HostKey != nil && r.Error == "" {
		return nil, fmt.Errorf("%s は初めてのサーバーです。鍵の指紋 %s (%s) を利用者に確かめてもらい、trust に渡してつなぎ直してください",
			hit.Name, r.HostKey.Fingerprint, r.HostKey.KeyType)
	}
	if r.Error != "" {
		return nil, errors.New(r.Error)
	}
	return map[string]string{"server": r.ID, "name": hit.Name, "home": r.Home}, nil
}

// status は、スマホの待ち受けの様子と、つないでいるサーバー
func (c *client) status() (any, error) {
	var phone, servers any
	if err := c.call(&phone, "PhoneState"); err != nil {
		return nil, err
	}
	if err := c.call(&servers, "Servers"); err != nil {
		return nil, err
	}
	return map[string]any{"phone": phone, "servers": servers}, nil
}

// listen は、スマホの待ち受け (lan か public) を切り替える。public の port が 0 なら、いまの設定のまま
func (c *client) listen(route string, on bool, port int) (any, error) {
	var st struct {
		Settings struct {
			PublicPort int `json:"publicPort"`
		} `json:"settings"`
	}
	var out any
	switch route {
	case "lan":
		return out, c.call(&out, "SetPhoneLAN", on)
	case "public":
		if port == 0 {
			if err := c.call(&st, "PhoneState"); err != nil {
				return nil, err
			}
			port = st.Settings.PublicPort
		}
		return out, c.call(&out, "SetPhonePublic", on, port)
	}
	return nil, errors.New(`route は "public" か "lan" です`)
}
