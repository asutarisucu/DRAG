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

## 置き場所

DRAG の設定と記録は `%AppData%\DRAG` に置きます。
サーバーの鍵は、そこの `known_hosts` に OpenSSH と同じ形で書きます。
