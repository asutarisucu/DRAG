# DRAG の PC アプリ

Wails v2 (Go) と React + TypeScript + Tailwind CSS で作っています。
画面の形は `mock/` のモックに合わせます。

## ビルド

Go と Node.js と Wails v2 の CLI が要ります。

```
wails build
```

`build/bin/DRAG.exe` ができます。

AI エージェント用の `dragctl.exe` は別に作ります。

```
go build -o build/bin/dragctl.exe ./cmd/dragctl
```

## AI エージェントから使う

`dragctl.exe` は、PC で動いている DRAG を AI エージェントから操作する道具です。
MCP のサーバーとして動かすか、同じ操作をコマンドで呼びます。
DRAG を起動すると、設定フォルダに窓口のポートと合言葉を書いた `agent.json` ができ、`dragctl.exe` はそれを読んでつなぎます。

Claude Code には、次のように MCP のサーバーとして登録します。

```
claude mcp add --scope user drag -- D:\dev\DRAG\app\build\bin\dragctl.exe mcp
```

できることは、スマホの登録 URL を出す、スマホの待ち受けの切り替え、登録済みのスマホと履歴、
保存済みのサーバーへの接続、コマンドの実行、ファイルの読み書き、sudo モードです。
エージェントが登録 URL を出すと、PC の画面に知らせが出て、スマホの履歴に残ります。

出先のスマホから使うには、直接公開を ON にしておき、エージェントに直接公開の登録 URL (`pair public`) を出してもらいます。
エージェントのリモート接続で届くのは会話だけで、スマホの通信は直接公開の経路を通ります。

コマンドの一覧は `dragctl.exe` を引数なしで動かすと出ます。

## 開発

```
wails dev
```

`http://localhost:34115` をブラウザーで開くと、Go の側とつながった画面をブラウザーでも触れます。

## テスト

`internal/remote` のテストは、検証用のサーバー (`dev/testserver`) を起動してから、接続先を渡して動かします。

```
cd dev/testserver && docker compose up -d --build
cd app && DRAG_TEST_SSH=127.0.0.1:2222 go test ./...
```

画面の通しの確認は、検証用のサーバーと `wails dev` を起動してから動かします。
接続が切れたときを試すため、途中で検証用のサーバーの sshd のプロセスを止めます。

```
cd app/frontend && node e2e/smoke.mjs
```

`dragctl` のテストは、`wails dev` を動かしたまま、開発用の DRAG に向けて動かします。

```
cd app && DRAG_TEST_AGENT=1 go test ./cmd/dragctl/
go build -o build/bin/dragctl.exe ./cmd/dragctl && cd frontend && node e2e/agent.mjs
```

## 置き場所

DRAG の設定と記録は `%AppData%\DRAG` に置きます。
サーバーの鍵は、そこの `known_hosts` に OpenSSH と同じ形で書きます。
保存済みのサーバーは `servers.json` に置き、パスワードは書きません。
画面の設定と区画の大きさは、画面の localStorage に置きます。
