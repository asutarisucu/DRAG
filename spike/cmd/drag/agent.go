//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/coder/websocket"
)

// エージェントは PC アプリやブラウザからの要求を受け、保持役につなぐ。
// シェルは持たないので、入れ替えても落ちてもセッションは残る

var idPattern = regexp.MustCompile(`^[A-Z2-7]{26}$`)

func runAgent() error {
	if err := os.MkdirAll(runDir(), 0o700); err != nil {
		return err
	}
	sock := filepath.Join(dragDir(), "agent.sock")
	os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	os.Chmod(sock, 0o600)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, version)
	})
	mux.HandleFunc("GET /sessions", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(listSessions())
	})
	mux.HandleFunc("POST /sessions", func(w http.ResponseWriter, r *http.Request) {
		id, err := spawnHolder()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, id)
	})
	mux.HandleFunc("GET /sessions/{id}", attach)
	// 応答してから終わる。サービスとして動いていれば systemd が起動し直す
	mux.HandleFunc("POST /restart", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "bye")
		time.AfterFunc(100*time.Millisecond, func() { os.Exit(0) })
	})
	return http.Serve(ln, mux)
}

func listSessions() []string {
	ids := []string{}
	socks, _ := filepath.Glob(filepath.Join(runDir(), "*.sock"))
	for _, s := range socks {
		c, err := net.Dial("unix", s)
		if err != nil {
			// 保持役が残っていない
			os.Remove(s)
			continue
		}
		c.Close()
		ids = append(ids, strings.TrimSuffix(filepath.Base(s), ".sock"))
	}
	return ids
}

func spawnHolder() (string, error) {
	id := rand.Text()
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	log, err := os.OpenFile(filepath.Join(runDir(), id+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", err
	}
	defer log.Close()
	cmd := exec.Command(exe, "hold", id)
	cmd.Stdout, cmd.Stderr = log, log
	// 新しいセッションにして、エージェントの端末やシグナルから切り離す
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go cmd.Wait()

	sock := filepath.Join(runDir(), id+".sock")
	for range 50 {
		if c, err := net.Dial("unix", sock); err == nil {
			c.Close()
			return id, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return "", fmt.Errorf("holder %s did not start", id)
}

// WebSocket の1本を保持役への接続1本につなぐ。
// バイナリのメッセージはキー入力、テキストは {"cols":..,"rows":..} の大きさの変更
func attach(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	hc, err := net.Dial("unix", filepath.Join(runDir(), id+".sock"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer hc.Close()
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer ws.CloseNow()
	ws.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer hc.Close()
		for {
			typ, p, err := ws.Read(ctx)
			if err != nil {
				return
			}
			if typ == websocket.MessageBinary {
				writeFrame(hc, frameInput, p)
				continue
			}
			var sz struct{ Cols, Rows uint16 }
			if json.Unmarshal(p, &sz) == nil && sz.Cols > 0 && sz.Rows > 0 {
				var b [4]byte
				binary.BigEndian.PutUint16(b[0:], sz.Cols)
				binary.BigEndian.PutUint16(b[2:], sz.Rows)
				writeFrame(hc, frameResize, b[:])
			}
		}
	}()

	for {
		typ, p, err := readFrame(hc)
		if err != nil {
			ws.Close(websocket.StatusGoingAway, "holder closed")
			return
		}
		switch typ {
		case frameOutput:
			if err := ws.Write(ctx, websocket.MessageBinary, p); err != nil {
				return
			}
		case frameExit:
			ws.Close(websocket.StatusNormalClosure, fmt.Sprintf("exit %d", int32(binary.BigEndian.Uint32(p))))
			return
		}
	}
}
