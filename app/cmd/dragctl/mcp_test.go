package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// 動いている開発用の DRAG (wails dev) につなぐテスト。DRAG_TEST_AGENT があるときだけ動く
func needAgent(t *testing.T) string {
	if os.Getenv("DRAG_TEST_AGENT") == "" {
		t.Skip("DRAG_TEST_AGENT が無い")
	}
	bin := filepath.Join(t.TempDir(), "dragctl.exe")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	return bin
}

// 古い形 (initialize で始める) のクライアントにも応える。Claude Code などは今この形で話す
func TestMCPLegacyInitialize(t *testing.T) {
	bin := needAgent(t)
	cmd := exec.Command(bin, "-dev", "mcp")
	in, _ := cmd.StdinPipe()
	outPipe, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	out := bufio.NewScanner(outPipe)
	out.Buffer(nil, 4<<20)
	send := func(v string) { in.Write([]byte(v + "\n")) }
	recv := func() map[string]any {
		done := make(chan bool, 1)
		var m map[string]any
		go func() { done <- out.Scan() }()
		select {
		case ok := <-done:
			if !ok {
				t.Fatal("dragctl が終わった")
			}
		case <-time.After(10 * time.Second):
			t.Fatal("返事が来ない")
		}
		json.Unmarshal(out.Bytes(), &m)
		return m
	}
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	init := recv()
	res, _ := init["result"].(map[string]any)
	if res["protocolVersion"] != "2025-06-18" || !strings.Contains(res["instructions"].(string), "直接公開") {
		t.Fatalf("%v", init)
	}
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	list := recv()
	tools, _ := list["result"].(map[string]any)["tools"].([]any)
	names := []string{}
	for _, x := range tools {
		names = append(names, x.(map[string]any)["name"].(string))
	}
	if len(names) != 14 || !strings.Contains(strings.Join(names, ","), "drag_phone_pair_url") {
		t.Fatalf("%v", names)
	}
	send(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"drag_status","arguments":{}}}`)
	call := recv()
	r := call["result"].(map[string]any)
	text := r["content"].([]any)[0].(map[string]any)["text"].(string)
	if r["isError"] == true || !strings.Contains(text, `"phone"`) || r["structuredContent"] != nil {
		t.Fatalf("%v", call)
	}
	// 道具の中の失敗は、isError の結果で返す
	send(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"drag_exec","arguments":{"server":"nowhere","command":"true"}}}`)
	bad := recv()["result"].(map[string]any)
	if bad["isError"] != true || !strings.Contains(bad["content"].([]any)[0].(map[string]any)["text"].(string), "nowhere につながっていません") {
		t.Fatalf("%v", bad)
	}
}

// 新しい形のクライアント (go-sdk の Client) からも使える
func TestMCPWithSDKClient(t *testing.T) {
	bin := needAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	sess, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.Command(bin, "-dev", "mcp")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	tools, err := sess.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 14 {
		t.Fatalf("%v %v", tools, err)
	}
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "drag_phone_devices", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("%+v %v", res, err)
	}
	if _, ok := res.Content[0].(*mcp.TextContent); !ok {
		t.Fatalf("%#v", res.Content)
	}
}
