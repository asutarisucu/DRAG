package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"rsc.io/qr"

	"github.com/asutarisucu/DRAG/app/internal/phone"
	"github.com/asutarisucu/DRAG/app/internal/secret"
)

// スマホの待ち受け。スマホの画面は PC アプリと同じものを出し、Go の呼び出しは WebSocket で受ける

// phoneMethods は、スマホの画面から呼べるメソッド。PC のファイルや窓を使うもの、端末の管理は入れない
var phoneMethods = map[string]bool{
	"Connect": true, "Reconnect": true, "Probe": true, "SavedServers": true, "Servers": true,
	"ListDir": true, "NewEntry": true, "RenameEntry": true, "PasteEntry": true, "TrashEntry": true, "RestoreEntry": true, "Stat": true,
	"OpenShell": true, "ShellReplay": true, "ShellCwd": true, "ShellInput": true, "ShellResize": true, "CloseShell": true, "OtherShells": true,
	"Sessions": true, "EnableSudo": true, "DisableSudo": true,
	"OpenText": true, "SaveText": true, "ReadImage": true, "LogTail": true, "LogFollow": true, "LogStop": true,
	"ArchiveOpen": true, "ArchiveRead": true, "ArchiveClose": true,
	"History": true, "Processes": true, "Mounts": true, "BigDirs": true, "Network": true,
	"TrashList": true, "TrashDelete": true, "DirUsage": true,
	"Services": true, "ServiceAction": true, "Docker": true, "DockerAction": true, "DockerMounts": true, "DockerVolumeSizes": true, "DockerStats": true, "LogStream": true,
}

const (
	defaultPhonePort  = 8443
	defaultPublicPort = 443
)

// PhoneSettings はスマホの待ち受けの設定。PC の設定フォルダの phone.json に置く
type PhoneSettings struct {
	LAN  bool `json:"lan"`
	Port int  `json:"port"`
	// 直接公開。ルーターで PublicPort を PC へ転送しておく
	Public     bool `json:"public"`
	PublicPort int  `json:"publicPort"`
	// 直接公開の証明書。"domain" なら Domain の証明書を Cloudflare の DNS で取る。ほか (空も) は IP アドレスの証明書。
	// Cloudflare の API トークンは、ここではなく cloudflare.token に暗号化して置く
	Cert   string `json:"cert"`
	Domain string `json:"domain"`
}

func phoneSettingsPath() string { return filepath.Join(configDir(), "phone.json") }

func loadPhoneSettings() PhoneSettings {
	ps := PhoneSettings{LAN: true, Port: defaultPhonePort, PublicPort: defaultPublicPort}
	if b, err := os.ReadFile(phoneSettingsPath()); err == nil {
		json.Unmarshal(b, &ps)
	}
	if ps.Port <= 0 || ps.Port > 65535 {
		ps.Port = defaultPhonePort
	}
	if ps.PublicPort <= 0 || ps.PublicPort > 65535 {
		ps.PublicPort = defaultPublicPort
	}
	return ps
}

func savePhoneSettings(ps PhoneSettings) error {
	b, _ := json.MarshalIndent(ps, "", "  ")
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(phoneSettingsPath(), b, 0o600)
}

// phoneAddr は待ち受けるアドレス。DRAG_PHONE_LISTEN があればそれを使う。
// 試すときに 127.0.0.1 だけで待ち受け、Windows のファイアウォールの確認を出さないため
func phoneAddr(ps PhoneSettings) string {
	if v := os.Getenv("DRAG_PHONE_LISTEN"); v != "" {
		return v
	}
	return ":" + strconv.Itoa(ps.Port)
}

func cfTokenPath() string { return filepath.Join(configDir(), "cloudflare.token") }

// loadCFToken は保存した Cloudflare の API トークン。無ければ空
func loadCFToken() string {
	b, err := os.ReadFile(cfTokenPath())
	if err != nil {
		return ""
	}
	tok, err := secret.Unprotect(b)
	if err != nil {
		return ""
	}
	return string(tok)
}

func saveCFToken(tok string) error {
	b, err := secret.Protect([]byte(tok))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(cfTokenPath(), b, 0o600)
}

// publicConfig は直接公開の待ち受けの設定。自分のドメインなら Cloudflare で TXT レコードを置く
func publicConfig(ps PhoneSettings) (phone.PublicConfig, error) {
	cfg := phone.PublicConfig{Port: ps.PublicPort}
	if ps.Cert != "domain" {
		return cfg, nil
	}
	tok := loadCFToken()
	if tok == "" {
		return cfg, errors.New("Cloudflare の API トークンがありません")
	}
	cfg.Domain, cfg.DNS = ps.Domain, &phone.Cloudflare{Token: tok}
	return cfg, nil
}

// domainRe は証明書を取れるドメインの形。英小文字・数字・ハイフンのラベルを2つ以上、ドットでつなぐ
var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]*[a-z0-9]$`)

func (a *App) startPhone() {
	dist, _ := fs.Sub(assets, "frontend/dist")
	// 試すときは、組み込んだものではなくフォルダの画面を出す。画面だけを作り直せば、スマホの側に出る
	if dir := os.Getenv("DRAG_PHONE_ASSETS"); dir != "" {
		dist = os.DirFS(dir)
	}
	a.phone = phone.New(phone.Options{
		Dir:     filepath.Join(configDir(), "phone"),
		Assets:  dist,
		Call:    a.phoneCall,
		Changed: func() { runtime.EventsEmit(a.ctx, "phone:changed") },
	})
	ps := loadPhoneSettings()
	if ps.LAN {
		if err := a.phone.Start(phoneAddr(ps)); err != nil {
			runtime.LogWarningf(a.ctx, "スマホの待ち受けを始められません: %v", err)
		}
	}
	if ps.Public {
		cfg, err := publicConfig(ps)
		if err == nil {
			err = a.phone.StartPublic(cfg)
		}
		if err != nil {
			runtime.LogWarningf(a.ctx, "直接公開の待ち受けを始められません: %v", err)
		}
	}
}

// phoneCall は、スマホの画面からの呼び出しを、その端末の App のメソッドに渡す
func (a *App) phoneCall(dev phone.Device, method string, raw []json.RawMessage) (any, error) {
	if !phoneMethods[method] {
		return nil, fmt.Errorf("スマホからは使えません: %s", method)
	}
	m := reflect.ValueOf(&App{core: a.core, dev: dev.ID}).MethodByName(method)
	t := m.Type()
	if t.NumIn() != len(raw) {
		return nil, fmt.Errorf("%s の引数の数が違います", method)
	}
	in := make([]reflect.Value, len(raw))
	for i, r := range raw {
		v := reflect.New(t.In(i))
		if err := json.Unmarshal(r, v.Interface()); err != nil {
			return nil, fmt.Errorf("%s の引数を読めません: %w", method, err)
		}
		in[i] = v.Elem()
	}
	// 打っている途中の行は、呼び出しの前に組み立てておく
	lines := audit(dev, method, raw)
	out := m.Call(in)
	// 戻り値は Wails と同じく、なし・値・error・値と error のどれか
	var res any
	for _, o := range out {
		if o.Type() == reflect.TypeFor[error]() {
			if !o.IsNil() {
				return nil, o.Interface().(error)
			}
			continue
		}
		res = o.Interface()
	}
	if r, ok := res.(ConnectResult); ok && r.Error != "" {
		return res, nil
	}
	for _, l := range lines {
		a.phone.Record(dev, l)
	}
	return res, nil
}

// PhoneState は、設定の画面とスマホの登録の窓に出す待ち受けの様子
type PhoneState struct {
	Settings PhoneSettings `json:"settings"`
	Status   phone.Status  `json:"status"` // LAN 内
	Public   phone.Status  `json:"public"` // 直接公開
	// Cloudflare の API トークンを保存してあるか。トークンそのものは画面に渡さない
	HasToken bool `json:"hasToken"`
}

func (a *App) PhoneState() PhoneState {
	return PhoneState{Settings: loadPhoneSettings(), Status: a.phone.Status(phone.RouteLAN), Public: a.phone.Status(phone.RoutePublic), HasToken: loadCFToken() != ""}
}

// SetPhoneLAN は LAN での待ち受けを切り替え、設定に書く
func (a *App) SetPhoneLAN(on bool) (PhoneState, error) {
	ps := loadPhoneSettings()
	ps.LAN = on
	if err := savePhoneSettings(ps); err != nil {
		return a.PhoneState(), err
	}
	var err error
	if on {
		err = a.phone.Start(phoneAddr(ps))
	} else {
		a.phone.StopRoute(phone.RouteLAN)
	}
	return a.PhoneState(), err
}

// SetPhonePublic は直接公開を切り替え、設定に書く。ON にすると IP アドレスの証明書を取り始める。
// グローバル IP は Cloudflare (1.1.1.1) に聞いて調べる
func (a *App) SetPhonePublic(on bool, port int) (PhoneState, error) {
	if port <= 0 || port > 65535 {
		return a.PhoneState(), errors.New("ポートは 1 から 65535 までです")
	}
	ps := loadPhoneSettings()
	ps.Public, ps.PublicPort = on, port
	if err := savePhoneSettings(ps); err != nil {
		return a.PhoneState(), err
	}
	var err error
	if on {
		var cfg phone.PublicConfig
		if cfg, err = publicConfig(ps); err == nil {
			err = a.phone.StartPublic(cfg)
		}
	} else {
		a.phone.StopRoute(phone.RoutePublic)
	}
	return a.PhoneState(), err
}

// SetPhoneCert は直接公開の証明書の取り方を変える。kind は "ip" か "domain"。
// "domain" のときは、domain の証明書を Cloudflare の DNS で取る。token が空なら前に保存したトークンを使う。
// 公開しているなら、新しい取り方で取り直す
func (a *App) SetPhoneCert(kind, domain, token string) (PhoneState, error) {
	ps := loadPhoneSettings()
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	token = strings.TrimSpace(token)
	if kind == "domain" {
		if len(domain) > 253 || !domainRe.MatchString(domain) {
			return a.PhoneState(), errors.New("ドメインの形が違います")
		}
		if token == "" && loadCFToken() == "" {
			return a.PhoneState(), errors.New("Cloudflare の API トークンを入れてください")
		}
		if token != "" {
			if err := saveCFToken(token); err != nil {
				return a.PhoneState(), err
			}
		}
		ps.Domain = domain
	} else {
		kind = "ip"
	}
	ps.Cert = kind
	if err := savePhoneSettings(ps); err != nil {
		return a.PhoneState(), err
	}
	var err error
	if ps.Public {
		var cfg phone.PublicConfig
		if cfg, err = publicConfig(ps); err == nil {
			err = a.phone.StartPublic(cfg)
		}
	}
	return a.PhoneState(), err
}

// Pairing は、スマホで読む QR コード。Path は 1 マス 1 単位の SVG のパスで、Size は余白を含む一辺
type Pairing struct {
	URL     string `json:"url"`
	Expires int64  `json:"expires"` // Unix ミリ秒
	Path    string `json:"path"`
	Size    int    `json:"size"`
}

// NewPairing は経路 route ("lan" か "public") の使い捨てのトークンを作り、その URL の QR コードを返す
func (a *App) NewPairing(route string) (Pairing, error) {
	u, exp, err := a.phone.NewPairURL(route)
	if err != nil {
		return Pairing{}, err
	}
	code, err := qr.Encode(u, qr.M)
	if err != nil {
		return Pairing{}, err
	}
	// まわりに 4 マスの余白を取る
	const quiet = 4
	var b strings.Builder
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}
	return Pairing{URL: u, Expires: exp.UnixMilli(), Path: b.String(), Size: code.Size + 2*quiet}, nil
}

func (a *App) PhoneDevices() []phone.Device { return a.phone.Devices() }

func (a *App) PhoneHistory() []phone.Login { return a.phone.History() }

func (a *App) RevokePhone(id string) error {
	if id == "" {
		return errors.New("端末がありません")
	}
	return a.phone.Revoke(id)
}
