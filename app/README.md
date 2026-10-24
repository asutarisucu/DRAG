# DRAG の PC アプリ

Wails v2 (Go) と React + TypeScript + Tailwind CSS で作っています。
画面の形は `mock/` のモックに合わせます。

## ビルド

Go と Node.js と Wails v2 の CLI が要ります。

```
wails build
```

`build/bin/DRAG.exe` ができます。

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

## 置き場所

DRAG の設定と記録は `%AppData%\DRAG` に置きます。
サーバーの鍵は、そこの `known_hosts` に OpenSSH と同じ形で書きます。
保存済みのサーバーは `servers.json` に置き、パスワードは書きません。
画面の設定と区画の大きさは、画面の localStorage に置きます。
