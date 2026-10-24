package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"rsc.io/qr"

	"github.com/asutarisucu/DRAG/app/internal/phone"
)

// スマホの待ち受け。スマホの画面は PC アプリと同じものを出し、Go の呼び出しは WebSocket で受ける

// phoneMethods は、スマホの画面から呼べるメソッド。PC のファイルや窓を使うもの、端末の管理は入れない
var phoneMethods = map[string]bool{
	"Connect": true, "Reconnect": true, "Probe": true, "SavedServers": true, "Servers": true,
	"ListDir": true, "NewEntry": true, "RenameEntry": true, "PasteEntry": true, "TrashEntry": true, "RestoreEntry": true, "Stat": true,
	"OpenShell": true, "ShellReplay": true, "ShellCwd": true, "ShellInput": true, "ShellResize": true, "CloseShell": true,
	"Sessions": true, "EnableSudo": true, "DisableSudo": true,
	"OpenText": true, "SaveText": true, "ReadImage": true, "LogTail": true, "LogFollow": true, "LogStop": true,
	"ArchiveOpen": true, "ArchiveRead": true, "ArchiveClose": true,
}

const defaultPhonePort = 8443

// PhoneSettings はスマホの待ち受けの設定。PC の設定フォルダの phone.json に置く
type PhoneSettings struct {
	LAN  bool `json:"lan"`
	Port int  `json:"port"`
}

func phoneSettingsPath() string { return filepath.Join(configDir(), "phone.json") }

func loadPhoneSettings() PhoneSettings {
	ps := PhoneSettings{LAN: true, Port: defaultPhonePort}
	if b, err := os.ReadFile(phoneSettingsPath()); err == nil {
		json.Unmarshal(b, &ps)
	}
	if ps.Port <= 0 || ps.Port > 65535 {
		ps.Port = defaultPhonePort
	}
	return ps
}

// phoneAddr は待ち受けるアドレス。DRAG_PHONE_LISTEN があればそれを使う。
// 試すときに 127.0.0.1 だけで待ち受け、Windows のファイアウォールの確認を出さないため
func phoneAddr(ps PhoneSettings) string {
	if v := os.Getenv("DRAG_PHONE_LISTEN"); v != "" {
		return v
	}
	return ":" + strconv.Itoa(ps.Port)
}

func (a *App) startPhone() {
	dist, _ := fs.Sub(assets, "frontend/dist")
	a.phone = phone.New(phone.Options{
		Dir:     filepath.Join(configDir(), "phone"),
		Assets:  dist,
		Call:    a.phoneCall,
		Changed: func() { runtime.EventsEmit(a.ctx, "phone:changed") },
	})
	if ps := loadPhoneSettings(); ps.LAN {
		if err := a.phone.Start(phoneAddr(ps)); err != nil {
			runtime.LogWarningf(a.ctx, "スマホの待ち受けを始められません: %v", err)
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
	return res, nil
}

// PhoneState は、設定の画面とスマホの登録の窓に出す待ち受けの様子
type PhoneState struct {
	Settings PhoneSettings `json:"settings"`
	Status   phone.Status  `json:"status"`
}

func (a *App) PhoneState() PhoneState {
	return PhoneState{Settings: loadPhoneSettings(), Status: a.phone.Status()}
}

// SetPhoneLAN は LAN での待ち受けを切り替え、設定に書く
func (a *App) SetPhoneLAN(on bool) (PhoneState, error) {
	ps := loadPhoneSettings()
	ps.LAN = on
	b, _ := json.MarshalIndent(ps, "", "  ")
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return a.PhoneState(), err
	}
	if err := os.WriteFile(phoneSettingsPath(), b, 0o600); err != nil {
		return a.PhoneState(), err
	}
	var err error
	if on {
		err = a.phone.Start(phoneAddr(ps))
	} else {
		a.phone.Stop()
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

// NewPairing は使い捨てのトークンを作り、その URL の QR コードを返す。前のトークンは使えなくなる
func (a *App) NewPairing() (Pairing, error) {
	u, exp, err := a.phone.NewPairURL()
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
