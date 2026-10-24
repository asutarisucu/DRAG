package phone

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Device は登録したスマホ1台。資格情報そのものは持たず、ハッシュだけを置く
type Device struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Hash     string    `json:"hash"`
	Route    string    `json:"route"` // "lan"
	Created  time.Time `json:"created"`
	LastSeen time.Time `json:"lastSeen"`
	LastIP   string    `json:"lastIP"`
}

// Login はログインと端末の登録の履歴1件
type Login struct {
	Time   time.Time `json:"time"`
	Kind   string    `json:"kind"` // "pair" 登録 / "login" ログイン / "pair-failed" 期限切れか使用済みの QR / "denied" 登録していない端末 / "op" スマホからの操作
	Device string    `json:"device"`
	IP     string    `json:"ip"`
	Route  string    `json:"route"`
	What   string    `json:"what,omitempty"` // 操作の中身
}

// loginLimit は残す履歴の数。スマホで打ったコマンドも入るので多めにする
const loginLimit = 1000

// devices は登録したスマホとログインの履歴。dir の devices.json と logins.json に置く
type devices struct {
	dir string

	mu     sync.Mutex
	list   []Device
	logins []Login
}

func openDevices(dir string) *devices {
	d := &devices{dir: dir}
	if b, err := os.ReadFile(filepath.Join(dir, "devices.json")); err == nil {
		json.Unmarshal(b, &d.list)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "logins.json")); err == nil {
		json.Unmarshal(b, &d.logins)
	}
	return d
}

func (d *devices) save(name string, v any) error {
	if err := os.MkdirAll(d.dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(d.dir, name+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(d.dir, name))
}

func hashSecret(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// add は端末を登録し、cookie に入れる資格情報 "<ID>.<秘密>" を返す
func (d *devices) add(name, route, ip string) (string, Device, error) {
	secret := rand.Text() + rand.Text()
	now := time.Now()
	dev := Device{ID: rand.Text()[:12], Name: name, Hash: hashSecret(secret), Route: route, Created: now, LastSeen: now, LastIP: ip}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.list = append(d.list, dev)
	if err := d.save("devices.json", d.list); err != nil {
		d.list = d.list[:len(d.list)-1]
		return "", Device{}, err
	}
	return dev.ID + "." + secret, dev, nil
}

// check は cookie の資格情報から端末を探す
func (d *devices) check(cred string) (Device, bool) {
	id, secret, ok := strings.Cut(cred, ".")
	if !ok || secret == "" {
		return Device{}, false
	}
	want := hashSecret(secret)
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, dev := range d.list {
		if dev.ID == id && subtle.ConstantTimeCompare([]byte(dev.Hash), []byte(want)) == 1 {
			return dev, true
		}
	}
	return Device{}, false
}

func (d *devices) seen(id, ip string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.list {
		if d.list[i].ID == id {
			d.list[i].LastSeen = time.Now()
			d.list[i].LastIP = ip
		}
	}
	d.save("devices.json", d.list)
}

var errNoDevice = errors.New("その端末は登録されていません")

func (d *devices) remove(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := slices.IndexFunc(d.list, func(x Device) bool { return x.ID == id })
	if i < 0 {
		return errNoDevice
	}
	d.list = slices.Delete(d.list, i, i+1)
	return d.save("devices.json", d.list)
}

func (d *devices) all() []Device {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.list)
}

func (d *devices) log(l Login) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.logins = append(d.logins, l)
	if over := len(d.logins) - loginLimit; over > 0 {
		d.logins = slices.Delete(d.logins, 0, over)
	}
	d.save("logins.json", d.logins)
}

// history はログインの履歴を新しい順に返す
func (d *devices) history() []Login {
	d.mu.Lock()
	defer d.mu.Unlock()
	h := slices.Clone(d.logins)
	slices.Reverse(h)
	return h
}

// deviceName は User-Agent から「Android (Chrome)」のような名前を作る
func deviceName(ua string) string {
	sys := "ブラウザ"
	for _, k := range [][2]string{{"iPhone", "iPhone"}, {"iPad", "iPad"}, {"Android", "Android"}, {"Windows", "Windows"}, {"Mac OS", "Mac"}, {"Linux", "Linux"}} {
		if strings.Contains(ua, k[0]) {
			sys = k[1]
			break
		}
	}
	for _, k := range [][2]string{{"Edg/", "Edge"}, {"Firefox/", "Firefox"}, {"CriOS/", "Chrome"}, {"Chrome/", "Chrome"}, {"Safari/", "Safari"}} {
		if strings.Contains(ua, k[0]) {
			return sys + " (" + k[1] + ")"
		}
	}
	return sys
}
