package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/asutarisucu/DRAG/app/internal/phone"
	"github.com/asutarisucu/DRAG/app/internal/remote"
)

// AI エージェントの窓口。dragctl (MCP とコマンド) が、この PC で動いている DRAG を呼ぶ。
// 127.0.0.1 の空いているポートで待ち受け、ポートと合言葉を設定フォルダの agent.json に置く。
// 設定フォルダはこの利用者のものなので、同じ利用者のプロセスだけが合言葉を読める

// agentDev は、エージェントを1台の端末として扱うときの名前。sudo モードはこの端末のものになる
const agentDev = "agent"

// agentMethods は、エージェントから呼べるメソッド
var agentMethods = map[string]bool{
	"AgentPair": true, "PhoneState": true, "PhoneDevices": true, "PhoneHistory": true, "RevokePhone": true,
	"SetPhoneLAN": true, "SetPhonePublic": true,
	"Servers": true, "SavedServers": true, "Connect": true, "Disconnect": true, "EnableSudo": true, "DisableSudo": true,
	"Exec": true, "ListDir": true, "OpenText": true, "WriteText": true,
}

// agentInfo は agent.json の中身。dragctl はこれを読んでつなぐ
type agentInfo struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

func agentInfoPath() string { return filepath.Join(configDir(), "agent.json") }

// startAgent はエージェントの窓口を開く
func (a *App) startAgent() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		runtime.LogWarningf(a.ctx, "エージェントの窓口を開けません: %v", err)
		return
	}
	info := agentInfo{Port: ln.Addr().(*net.TCPAddr).Port, Token: rand.Text(), PID: os.Getpid()}
	b, _ := json.Marshal(info)
	if err := os.MkdirAll(configDir(), 0o700); err == nil {
		err = os.WriteFile(agentInfoPath(), b, 0o600)
	}
	if err != nil {
		ln.Close()
		runtime.LogWarningf(a.ctx, "agent.json を書けません: %v", err)
		return
	}
	a.agent = &http.Server{Handler: a.agentHandler(info.Token), ReadHeaderTimeout: 10 * time.Second, ErrorLog: log.New(io.Discard, "", 0)}
	go a.agent.Serve(ln)
}

func (a *App) stopAgent() {
	if a.agent == nil {
		return
	}
	a.agent.Close()
	os.Remove(agentInfoPath())
}

// agentHandler は POST /call で {"m": メソッド, "a": [引数]} を受け、{"r": 戻り値} か {"e": エラー} を返す
func (a *App) agentHandler(token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /call", func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var req struct {
			M string            `json:"m"`
			A []json.RawMessage `json:"a"`
		}
		// ファイルの書き込みで中身ごと届くので、上限は大きめにする
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20)).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := a.agentCall(req.M, req.A)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"e": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"r": res})
	})
	return mux
}

// agentCall は、エージェントからの呼び出しを、エージェントの端末の App のメソッドに渡す。
// スマホの登録と待ち受けにかかわる操作は、スマホの履歴に残す
func (a *App) agentCall(method string, raw []json.RawMessage) (any, error) {
	if !agentMethods[method] {
		return nil, fmt.Errorf("エージェントからは使えません: %s", method)
	}
	res, err := invoke(&App{core: a.core, dev: agentDev}, method, raw)
	if err != nil {
		return nil, err
	}
	if what := agentNote(method, raw); what != "" && a.phone != nil {
		a.phone.Note("agent", "", what)
	}
	return res, nil
}

// agentNote は、履歴に残すエージェントの操作の文。残さないものは空。スマホの登録 URL は AgentPair が自分で残す
func agentNote(method string, raw []json.RawMessage) string {
	arg := func(i int) string {
		var v any
		if i >= len(raw) || json.Unmarshal(raw[i], &v) != nil {
			return ""
		}
		return fmt.Sprint(v)
	}
	onOff := map[string]string{"true": "ON", "false": "OFF"}
	switch method {
	case "RevokePhone":
		return "スマホを無効にする " + arg(0)
	case "SetPhoneLAN":
		return "LAN 内の待ち受けを " + onOff[arg(0)]
	case "SetPhonePublic":
		return "直接公開を " + onOff[arg(0)]
	}
	return ""
}

// AgentPairing は、エージェントに渡すスマホの登録 URL
type AgentPairing struct {
	URL     string `json:"url"`
	Expires int64  `json:"expires"` // Unix ミリ秒
	Route   string `json:"route"`
}

// AgentPair は、AI エージェントにスマホの登録 URL を出す。route は "public" (直接公開) か "lan"。
// 出したことはスマホの履歴に残し、PC の画面にも知らせる
func (a *App) AgentPair(route string) (AgentPairing, error) {
	if route != phone.RouteLAN && route != phone.RoutePublic {
		return AgentPairing{}, errors.New(`route は "public" か "lan" です`)
	}
	u, exp, err := a.phone.NewPairURL(route)
	if err != nil {
		if route == phone.RoutePublic {
			err = fmt.Errorf("直接公開の登録 URL を出せません。直接公開を ON にしてください: %w", err)
		}
		return AgentPairing{}, err
	}
	a.phone.Note("agent", route, "スマホの登録 URL を発行")
	a.emit("agent:pair", route)
	return AgentPairing{URL: u, Expires: exp.UnixMilli(), Route: route}, nil
}

// Exec は、サーバーで端末を使わずにコマンドを1つ実行する。timeoutSec は 1〜600 秒で、0 なら 60 秒
func (a *App) Exec(serverID, command string, timeoutSec int) (remote.ExecResult, error) {
	s, err := a.server(serverID)
	if err != nil {
		return remote.ExecResult{}, err
	}
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	return s.Exec(command, time.Duration(min(timeoutSec, 600))*time.Second)
}

// WriteText はテキストのファイルを書く。無ければ作り、あれば中身を置き換える
func (a *App) WriteText(serverID, p, text string) error {
	s, err := a.server(serverID)
	if err != nil {
		return err
	}
	_, err = s.WriteText(p, text, 0, true)
	return err
}
