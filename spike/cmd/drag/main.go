//go:build linux

// drag はサーバーに置く実行ファイル。技術検証用の試作
//
//	drag agent      エージェントを動かす
//	drag hold <id>  セッションの保持役を動かす。エージェントが起動する
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// ビルドのときに -ldflags "-X main.version=..." で入れる
var version = "dev"

func dragDir() string { return filepath.Join(os.Getenv("HOME"), ".drag") }
func runDir() string  { return filepath.Join(dragDir(), "run") }

func main() {
	var err error
	switch {
	case len(os.Args) == 2 && os.Args[1] == "agent":
		err = runAgent()
	case len(os.Args) == 3 && os.Args[1] == "hold":
		err = runHold(os.Args[2])
	default:
		fmt.Fprintln(os.Stderr, "usage: drag agent | drag hold <id>")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
