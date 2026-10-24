package main

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCP のサーバー。道具の使い方は modelcontextprotocol/go-sdk@v1.8.0 mcp/server.go の AddTool に合わせる。
// 引数の型の jsonschema タグが説明になり、omitempty の付いたものは省ける (google/jsonschema-go@v0.4.3)

const instructions = `DRAG は、SSH の先のサーバーを GUI で扱う PC アプリ。この道具で、PC で動いている DRAG を操作する。
スマホを外から使うときは、直接公開 (route=public) の登録 URL を出して利用者に渡す。LAN の URL は家の中でしか開けない。
登録 URL は5分で切れ、1回しか使えない。開いた人が誰でも登録できるので、利用者本人にだけ渡す。
サーバーは drag_servers の ID (s1 など) か名前で指す`

type noArgs struct{}

type routeArgs struct {
	Route string `json:"route,omitempty" jsonschema:"public (直接公開。出先のスマホから開ける) か lan (家の中だけ)。省くと public"`
}

type listenArgs struct {
	Route string `json:"route" jsonschema:"public (直接公開) か lan (LAN 内)"`
	On    bool   `json:"on" jsonschema:"true で待ち受ける"`
	Port  int    `json:"port,omitempty" jsonschema:"public のときのポート。省くといまの設定のまま"`
}

type idArgs struct {
	ID string `json:"id" jsonschema:"drag_phone_devices の端末の ID"`
}

type historyArgs struct {
	Limit int `json:"limit,omitempty" jsonschema:"新しいものから何件返すか。省くと 50"`
}

type connectArgs struct {
	Name     string `json:"name" jsonschema:"保存済みのサーバーの名前かホスト"`
	Password string `json:"password,omitempty" jsonschema:"パスワードでつなぐサーバーのパスワード"`
	Trust    string `json:"trust,omitempty" jsonschema:"初めてのサーバーのとき、利用者が確かめた鍵の指紋 (SHA256:...)"`
}

type serverArgs struct {
	Server string `json:"server" jsonschema:"サーバーの ID (s1 など) か名前"`
}

type sudoArgs struct {
	Server   string `json:"server" jsonschema:"サーバーの ID (s1 など) か名前"`
	On       bool   `json:"on" jsonschema:"true で sudo モードを ON にする"`
	Password string `json:"password,omitempty" jsonschema:"ON にするときの sudo のパスワード"`
}

type execArgs struct {
	Server     string `json:"server" jsonschema:"サーバーの ID (s1 など) か名前"`
	Command    string `json:"command" jsonschema:"/bin/sh で実行するコマンド。端末は無く、標準入力は空"`
	TimeoutSec int    `json:"timeout_sec,omitempty" jsonschema:"時間切れまでの秒数 (1〜600)。省くと 60"`
}

type pathArgs struct {
	Server string `json:"server" jsonschema:"サーバーの ID (s1 など) か名前"`
	Path   string `json:"path" jsonschema:"サーバーの上の絶対パス"`
}

type writeArgs struct {
	Server  string `json:"server" jsonschema:"サーバーの ID (s1 など) か名前"`
	Path    string `json:"path" jsonschema:"サーバーの上の絶対パス"`
	Content string `json:"content" jsonschema:"書く中身。ファイルはこの中身に置き換わる"`
}

// tool は、引数を受けて DRAG を呼ぶ道具を1つ足す。
// 結果は JSON の文字で返し、structuredContent は付けない。配列や文字の structuredContent を読めない古いクライアントがあるため
func tool[In any](s *mcp.Server, name, desc string, run func(In) (any, error)) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc}, func(_ context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		out, err := run(in)
		if err != nil {
			return nil, nil, err
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
	})
}

func newMCPServer(c *client, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "drag", Title: "DRAG", Version: version}, &mcp.ServerOptions{Instructions: instructions})
	// 戻り値の無いメソッドは "ok" を返す
	call := func(method string, args ...any) (any, error) {
		var out any
		if err := c.call(&out, method, args...); err != nil {
			return nil, err
		}
		if out == nil {
			return "ok", nil
		}
		return out, nil
	}
	withServer := func(ref string, f func(id string) (any, error)) (any, error) {
		id, err := c.server(ref)
		if err != nil {
			return nil, err
		}
		return f(id)
	}

	tool(s, "drag_status", "スマホの待ち受け (LAN 内と直接公開の URL・証明書) と、DRAG がつないでいるサーバーを返す",
		func(noArgs) (any, error) { return c.status() })
	tool(s, "drag_phone_pair_url", "スマホの登録 URL を出す。5分で切れ、1回しか使えない。出したことは DRAG の履歴に残り、PC にも知らせが出る",
		func(a routeArgs) (any, error) {
			if a.Route == "" {
				a.Route = "public"
			}
			return call("AgentPair", a.Route)
		})
	tool(s, "drag_phone_listen", "スマホの待ち受けを切り替える。直接公開を ON にすると、Let's Encrypt で証明書を取ってから使えるようになる",
		func(a listenArgs) (any, error) { return c.listen(a.Route, a.On, a.Port) })
	tool(s, "drag_phone_devices", "登録済みのスマホの一覧",
		func(noArgs) (any, error) { return call("PhoneDevices") })
	tool(s, "drag_phone_revoke", "登録済みのスマホを無効にする。そのスマホの画面は切れる",
		func(a idArgs) (any, error) { return call("RevokePhone", a.ID) })
	tool(s, "drag_phone_history", "スマホの登録・ログイン・操作と、AI エージェントの操作の履歴を、新しい順に返す",
		func(a historyArgs) (any, error) {
			var list []any
			if err := c.call(&list, "PhoneHistory"); err != nil {
				return nil, err
			}
			if a.Limit <= 0 {
				a.Limit = 50
			}
			return list[:min(a.Limit, len(list))], nil
		})
	tool(s, "drag_servers", "DRAG がつないでいるサーバーと、保存済みのサーバー",
		func(noArgs) (any, error) {
			connected, err := call("Servers")
			if err != nil {
				return nil, err
			}
			saved, err := call("SavedServers")
			return map[string]any{"connected": connected, "saved": saved}, err
		})
	tool(s, "drag_connect", "保存済みのサーバーにつなぐ。つないだサーバーは PC の DRAG の画面にもタブで出る",
		func(a connectArgs) (any, error) { return c.connect(a.Name, a.Password, a.Trust) })
	tool(s, "drag_disconnect", "サーバーとの接続を切る。PC の DRAG の画面のタブも閉じる",
		func(a serverArgs) (any, error) {
			return withServer(a.Server, func(id string) (any, error) { return call("Disconnect", id) })
		})
	tool(s, "drag_sudo", "そのサーバーでの AI エージェントの sudo モードを切り替える。ON の間はコマンドとファイルの操作を sudo で行う",
		func(a sudoArgs) (any, error) {
			return withServer(a.Server, func(id string) (any, error) {
				if a.On {
					return call("EnableSudo", id, a.Password)
				}
				return call("DisableSudo", id)
			})
		})
	tool(s, "drag_exec", "サーバーでコマンドを1つ実行し、標準出力・標準エラー・終了コードを返す。対話するコマンドは使えない",
		func(a execArgs) (any, error) {
			return withServer(a.Server, func(id string) (any, error) { return call("Exec", id, a.Command, a.TimeoutSec) })
		})
	tool(s, "drag_list_dir", "サーバーのフォルダの中を返す",
		func(a pathArgs) (any, error) {
			return withServer(a.Server, func(id string) (any, error) { return call("ListDir", id, a.Path) })
		})
	tool(s, "drag_read_file", "サーバーのテキストのファイルを読む",
		func(a pathArgs) (any, error) {
			return withServer(a.Server, func(id string) (any, error) { return call("OpenText", id, a.Path) })
		})
	tool(s, "drag_write_file", "サーバーのテキストのファイルを書く。無ければ作り、あれば中身を置き換える",
		func(a writeArgs) (any, error) {
			return withServer(a.Server, func(id string) (any, error) { return call("WriteText", id, a.Path, a.Content) })
		})
	return s
}
