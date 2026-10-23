# DRAG 技術検証の結果

企画書の7章に挙げた5つの検証の結果を、終わった順に残します。
試作のコードは `spike/` に、検証用のサーバーは `dev/testserver/` にあります。

| 検証 | 状態 |
|---|---|
| エージェントを入れ替えても、シェルが切れずに残るか | 残る (2026-10-24) |
| エージェントの転送から起動までにかかる時間 | ローカルで 0.4 秒。実際の回線では転送で決まる (2026-10-24) |
| PC の幅のターミナルを、スマホで横スクロールさせて表示できるか | できる。縦のスクロールは自前の処理が要る。実機は未確認 (2026-10-24) |
| Wails の WebView2 の上で、xterm.js に日本語入力が通るか | 未 |
| IP アドレスの証明書を、エージェントから取得して更新し続けられるか | 未 |

## 検証の環境

検証用のサーバーは Docker のコンテナで、Ubuntu 24.04、systemd 255、OpenSSH 9.6p1 です。
systemd を PID 1 で動かし、エージェントを本番と同じくサービスとして登録しています。
PC アプリの代わりに、Windows で動かす `spike/cmd/probe` が SSH でつなぎ、エージェントを入れて試します。

```
cd dev/testserver && docker compose up -d --build
cd spike && go run ./cmd/probe
```

## シェルは保持役のプロセスに持たせ、unit には KillMode=process を書く

シェルと PTY は、セッションごとに起動する保持役のプロセスが持ちます。
エージェントはシェルを持たず、保持役と unix ソケットでつなぐだけです。
保持役は直近の出力を 512 KiB まで持ち、つなぎ直したクライアントに最初に送ります。

シェルに変数とカレントディレクトリを持たせてから次の4つを試し、どれも想定どおりでした。

| 試したこと | 結果 |
|---|---|
| 版を上げた実行ファイルを送り、エージェントを起動し直す | 変数、カレントディレクトリ、シェルの PID が残る。それまでの出力も再送される |
| エージェントを kill -9 で落とし、systemd に起動し直させる | 同上 |
| サービスにせず SSH から起動したエージェントを落とし、起動し直す | 同上 |
| 対照として、unit の KillMode を既定の control-group にして再起動する | 保持役も止められ、セッションが消える |

systemd のサービスにするときは、unit に `KillMode=process` を書きます。
既定の control-group では、サービスを再起動したときに、サービスの cgroup にあるプロセスが全部止められます。
Ubuntu 24.04 の ssh.service も、ログイン中のセッションを残すために `KillMode=process` にしています。
この設定では、起動のたびに journal に次の行が出ます。
動作には影響しませんでした。

```
drag-agent.service: Found left-over process 198 (bash) in control group while starting unit. Ignoring.
```

エージェントの入れ替えに sudo は要りません。
新しい実行ファイルを別名で送ってから名前を変え、エージェントに終了を頼みます。
unit の `Restart=always` で、systemd が新しい版を起動します。
動いている保持役は、古い実行ファイルのまま動き続けます。

試した unit は次のとおりです。

```ini
[Service]
User=asuta
ExecStart=/home/asuta/.drag/bin/drag agent
Restart=always
KillMode=process
```

vi を開いたままつなぎ直したときの表示は、スマホの表示の試験で確かめました。

### サービスから起動するシェルには、ロケールを補う

systemd から起動したエージェントには、LANG などのロケールの環境変数がありません。
そのまま起動したシェルで `echo 日本語` と打つと、bash は打った行を `echo \346\227\245\346\234\254\350\252\236` と表示しました。
SSH でログインしたときは、sshd の PAM の pam_env が `/etc/environment` と `/etc/default/locale` を読んで設定しています。
試作のエージェントは、LANG と LC_ALL が無いときに `/etc/default/locale` か `/etc/locale.conf` を読んで補います。
どちらも無ければ `LANG=C.UTF-8` にします。
補ったあとは、打った日本語がそのまま表示されました。

## 配置にかかる時間は回線の速さで決まり、gzip で送ると半分以下になる

ローカルのコンテナで測った時間です。

| 項目 | 時間 |
|---|---|
| SSH の接続 (パスワード認証) | 402 ms |
| 初回。転送、unit の登録、起動して応答するまで | 363 ms (うち転送 38 ms) |
| 入れ替え。転送、終了、systemd による起動、応答するまで | 357 ms (うち転送 31 ms) |

ローカルの転送は 7.3 MB を 38 ms で送っていて、実際の回線よりずっと速い条件です。
実際の時間は、ほぼ実行ファイルの大きさを回線の上りの速さで割った値になります (見立て)。

| 実行ファイル (`-ldflags "-s -w"`) | そのまま | gzip -9 |
|---|---|---|
| amd64 | 7.3 MB | 3.1 MB |
| arm64 | 6.7 MB | 2.7 MB |

上り 10 Mbps の回線では、amd64 をそのまま送ると約 5.8 秒、gzip で送ると約 2.4 秒の計算です。
検証用のコンテナ (Ubuntu 24.04 の最小構成) には gzip がありましたが、xz はありませんでした。
配置では gzip で圧縮して送り、サーバーで展開します。

## スマホでは PC の幅のまま横にスクロールでき、縦のスクロールはページ側で受ける

xterm.js 6.0.0 を桁数 120、行数 30 で開き、外側の要素を `overflow-x: auto` にして横にスクロールさせました。
Chrome のスマホの表示 (390 × 844、タッチあり) で、`spike/phonetest/phone.mjs` が次を確かめ、全部通りました。

| 試したこと | 結果 |
|---|---|
| 120 桁を描く | ターミナルは幅 840 px で描かれ、ページ全体は 390 px のまま横にはみ出さない |
| 横にスワイプする | 右端の 450 px まで動く |
| 縦にスワイプする | 300 px で 23 行戻る。横の位置は動かない |
| ターミナルをタップする | xterm.js の入力欄にフォーカスが移る |
| 長いコマンドを打つ | 入力位置が見える位置まで、ブラウザが横に動かす |
| vi を開いたまま別のページでつなぎ直す | 代替画面のまま、30 行とも元のページと同じ内容になる |

縦のスクロールは、xterm.js だけでは動きませんでした。
xterm.js 6.0.0 はタッチでスクロールバックを動かす処理を持っていません。
`src/browser/Types.ts` に `handleTouchStart` の宣言だけがあり、実装はありません。
VS Code から持ってきた `src/vs/base/browser/touch.ts` の Gesture も、登録する呼び出しがありません。
試作のページでは、指の動きが縦向きのときだけ `touchmove` を受け、`term.scrollLines()` で行を送っています。
横向きのときはブラウザの横スクロールに任せます。
指を離したあとに慣性で流れる動きは、まだ入れていません。

試した方法は次のとおりです。
`drag` はあらかじめ `GOOS=linux go build ./cmd/drag` でビルドし、`docker cp` でコンテナの `/usr/local/bin/` に置きます。

```
docker compose exec -d -u asuta -e HOME=/home/asuta -e SHELL=/bin/bash -e DRAG_LISTEN=0.0.0.0:8080 testserver drag agent
cd spike/phonetest && npm install && node phone.mjs
```

Chrome DevTools Protocol の `Input.synthesizeScrollGesture` では、素の横スクロールの要素でも動きませんでした。
試験では `Input.dispatchTouchEvent` でタッチを1つずつ送っています。

分かっていないことが2つあります。

- iPhone の Safari と Android の Chrome の実機では試していません。Chrome のスマホの表示で試しただけです
- 再送は直近の 512 KiB だけなので、全画面のアプリを開いてからそれ以上の出力があると、代替画面に切り替えた出力が再送から外れます。そのときの表示は試していません
