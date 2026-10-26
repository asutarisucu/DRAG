package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/asutarisucu/DRAG/app/internal/phone"
	"github.com/asutarisucu/DRAG/app/internal/remote"
)

// スマホからの操作の記録。ログインの履歴と同じところに残し、PC の「スマホを登録」の窓で見る。
// 値を読むだけの呼び出しは残さない。パスワードは残さない。
// シェルに打った行は、シェル自身が前面にいて、コマンドとして読むときだけ残す

// lineBufs は、スマホからシェルに打っている途中の行。Enter で1行として残す
var lineBufs = struct {
	sync.Mutex
	m map[string][]rune
}{m: map[string][]rune{}}

// audit は、記録に残す呼び出しなら中身の文を返す。残さないものは空。
// シェルに打った行は、打った先をシェルに送る前に確かめるので、呼び出しより前に呼ぶ
func (a *App) audit(dev phone.Device, method string, raw []json.RawMessage) []string {
	// 引数の数は、このあと呼ぶときに確かめる。ここでは足りなくても読まずに進む
	decode := func(i int, v any) {
		if i < len(raw) {
			json.Unmarshal(raw[i], v)
		}
	}
	arg := func(i int) string {
		var v any
		decode(i, &v)
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
	switch method {
	case "Connect":
		var req ConnectRequest
		decode(0, &req)
		return []string{fmt.Sprintf("接続 %s@%s", req.User, req.Host)}
	case "AddVPN":
		var in VPNInput
		decode(0, &in)
		return []string{fmt.Sprintf("VPN を追加 %s", in.Name)}
	case "ServiceAction":
		return []string{fmt.Sprintf("サービス %s %s", arg(2), arg(1))}
	case "DockerAction":
		return []string{fmt.Sprintf("docker %s %s", arg(1), arg(2))}
	case "NewEntry":
		return []string{fmt.Sprintf("作成 %s/%s", arg(1), arg(2))}
	case "RenameEntry":
		return []string{fmt.Sprintf("名前の変更 %s → %s", arg(1), arg(2))}
	case "PasteEntry":
		verb := "複製"
		if arg(3) == "true" {
			verb = "移動"
		}
		return []string{fmt.Sprintf("%s %s → %s", verb, arg(1), arg(2))}
	case "TrashEntry":
		return []string{fmt.Sprintf("ゴミ箱へ %s", arg(1))}
	case "RestoreEntry":
		var it remote.TrashItem
		decode(1, &it)
		return []string{fmt.Sprintf("元に戻す %s", it.From)}
	case "TrashDelete":
		var names []string
		decode(1, &names)
		if len(names) == 0 {
			return []string{"ゴミ箱を空にする"}
		}
		return []string{"ゴミ箱から完全に削除 " + strings.Join(names, ", ")}
	case "SaveText":
		return []string{fmt.Sprintf("保存 %s", arg(1))}
	case "EnableSudo":
		return []string{"sudo モードを ON"}
	case "DisableSudo":
		return []string{"sudo モードを OFF"}
	case "ShellInput":
		lines := typedLines(dev.ID+"/"+arg(0), arg(1))
		if len(lines) == 0 {
			return nil
		}
		// sudo や ssh のパスワードの問い合わせに打った行は、問い合わせたコマンドが前面にいるので残さない。
		// まとめて何行も送ったときは、2行目からを読むのがシェルか分からないので、1行目だけを残す
		if sh := a.shell(arg(0)); sh == nil || !sh.AtPrompt() {
			return nil
		}
		return lines[:1]
	}
	return nil
}

// typedLines は、シェルに送ったキーを行ごとに組み立て、Enter で終わった行を返す。
// 消すキーは1文字消し、矢印などのエスケープの並びと制御文字は捨てる。補完や履歴で出した文字は入らない
func typedLines(key, data string) []string {
	lineBufs.Lock()
	defer lineBufs.Unlock()
	buf := lineBufs.m[key]
	var done []string
	rs := []rune(data)
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; {
		case r == '\r' || r == '\n':
			if l := strings.TrimSpace(string(buf)); l != "" {
				done = append(done, "コマンド "+l)
			}
			buf = buf[:0]
		case r == 0x7f || r == '\b':
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
			}
		case r == 0x1b:
			// ESC [ ... 英字 か ESC O 英字 を読み飛ばす
			if i+1 < len(rs) && (rs[i+1] == '[' || rs[i+1] == 'O') {
				i += 2
				for i < len(rs) && !(rs[i] >= '@' && rs[i] <= '~') {
					i++
				}
			}
		case r == 0x03 || r == 0x15:
			// Ctrl+C と Ctrl+U は行を捨てる
			buf = buf[:0]
		case r < 0x20:
		default:
			buf = append(buf, r)
		}
	}
	lineBufs.m[key] = buf
	return done
}
