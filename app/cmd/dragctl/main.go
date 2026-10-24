// dragctl は、AI エージェントから DRAG を操作するための道具。
// MCP のサーバー (dragctl mcp) と、同じ操作のコマンドを持つ。PC で動いている DRAG の窓口につなぐ
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "0.1.0"

const usage = `dragctl は、PC で動いている DRAG を AI エージェントから操作する。

  dragctl [-dev] <コマンド>

  mcp                                   MCP のサーバーとして標準入出力で動く
  status                                スマホの待ち受けと、つないでいるサーバー
  pair [public|lan]                     スマホの登録 URL を出す (省くと public)
  listen <public|lan> <on|off> [ポート]  スマホの待ち受けを切り替える
  devices                               登録済みのスマホ
  revoke <ID>                           スマホを無効にする
  history [件数]                         スマホとエージェントの操作の履歴
  servers                               つないでいるサーバーと、保存済みのサーバー
  connect <名前> [-trust 指紋]            保存済みのサーバーにつなぐ。パスワードは標準入力の1行目
  disconnect <サーバー>
  sudo <サーバー> <on|off>                sudo モード。on のパスワードは標準入力の1行目
  exec <サーバー> <コマンド>...            コマンドを実行し、出力と終了コードをそのまま返す
  ls <サーバー> <パス>
  cat <サーバー> <パス>
  write <サーバー> <パス>                  標準入力の中身でファイルを書く

  -dev は、開発用の DRAG (wails dev) につなぐ
`

func main() {
	dev := flag.Bool("dev", false, "開発用の DRAG (wails dev) につなぐ")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}
	c, err := open(*dev)
	if err != nil {
		fail(err)
	}
	if args[0] == "mcp" {
		if err := newMCPServer(c, version).Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			fail(err)
		}
		return
	}
	os.Exit(run(c, args[0], args[1:]))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "dragctl:", err)
	os.Exit(1)
}

// firstLine は標準入力の1行目。パスワードを渡すのに使う
func firstLine() string {
	l, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(l, "\r\n")
}

// run はコマンドを1つ実行し、終了コードを返す
func run(c *client, cmd string, args []string) int {
	need := func(n int) {
		if len(args) < n {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
	}
	var out any
	var err error
	call := func(method string, a ...any) {
		err = c.call(&out, method, a...)
	}
	withServer := func(ref string, f func(id string)) {
		var id string
		if id, err = c.server(ref); err == nil {
			f(id)
		}
	}
	switch cmd {
	case "status":
		out, err = c.status()
	case "pair":
		route := "public"
		if len(args) > 0 {
			route = args[0]
		}
		call("AgentPair", route)
	case "listen":
		need(2)
		port := 0
		if len(args) > 2 {
			port, _ = strconv.Atoi(args[2])
		}
		out, err = c.listen(args[0], args[1] == "on", port)
	case "devices":
		call("PhoneDevices")
	case "revoke":
		need(1)
		call("RevokePhone", args[0])
	case "history":
		limit := 50
		if len(args) > 0 {
			limit, _ = strconv.Atoi(args[0])
		}
		var list []any
		if err = c.call(&list, "PhoneHistory"); err == nil {
			out = list[:min(max(limit, 0), len(list))]
		}
	case "servers":
		var connected, saved any
		if err = c.call(&connected, "Servers"); err == nil {
			err = c.call(&saved, "SavedServers")
		}
		out = map[string]any{"connected": connected, "saved": saved}
	case "connect":
		need(1)
		fs := flag.NewFlagSet("connect", flag.ExitOnError)
		trust := fs.String("trust", "", "初めてのサーバーの鍵の指紋")
		fs.Parse(args[1:])
		out, err = c.connect(args[0], firstLine(), *trust)
	case "disconnect":
		need(1)
		withServer(args[0], func(id string) { call("Disconnect", id) })
	case "sudo":
		need(2)
		withServer(args[0], func(id string) {
			if args[1] == "on" {
				call("EnableSudo", id, firstLine())
			} else {
				call("DisableSudo", id)
			}
		})
	case "exec":
		need(2)
		var r struct {
			Stdout    string `json:"stdout"`
			Stderr    string `json:"stderr"`
			Code      int    `json:"code"`
			TimedOut  bool   `json:"timedOut"`
			Truncated bool   `json:"truncated"`
		}
		withServer(args[0], func(id string) { err = c.call(&r, "Exec", id, strings.Join(args[1:], " "), 0) })
		if err != nil {
			break
		}
		fmt.Print(r.Stdout)
		fmt.Fprint(os.Stderr, r.Stderr)
		if r.TimedOut {
			fmt.Fprintln(os.Stderr, "dragctl: 時間切れで止めました")
		}
		if r.Truncated {
			fmt.Fprintln(os.Stderr, "dragctl: 出力が多いので頭だけを出しました")
		}
		return r.Code & 0xff
	case "ls":
		need(2)
		withServer(args[0], func(id string) { call("ListDir", id, args[1]) })
	case "cat":
		need(2)
		var f struct {
			Text string `json:"text"`
		}
		withServer(args[0], func(id string) { err = c.call(&f, "OpenText", id, args[1]) })
		if err == nil {
			fmt.Print(f.Text)
			return 0
		}
	case "write":
		need(2)
		b, rerr := io.ReadAll(os.Stdin)
		if rerr != nil {
			fail(rerr)
		}
		withServer(args[0], func(id string) { call("WriteText", id, args[1], string(b)) })
	default:
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dragctl:", err)
		return 1
	}
	if out != nil {
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
	}
	return 0
}
