# DRAG 技術検証の結果

企画書の7章に挙げた検証の結果を、終わった順に残します。
試作のコードは `spike/` に、検証用のサーバーは `dev/testserver/` にあります。

| 検証 | 状態 |
|---|---|
| Wails の WebView2 の上で、xterm.js に日本語入力が通るか | Meltype では通る。Microsoft IME と Google 日本語入力は未確認 (2026-10-24) |
| PC の幅のターミナルを、スマホで横スクロールさせて表示できるか | できる。縦のスクロールは自前の処理が要る。実機は未確認 (2026-10-24) |
| 前面で動いているコマンドを、SSH の別のチャネルから調べて切り出せるか | 未 |
| sudo モードで、ファイルの操作とコマンドを sudo の権限で行えるか | 未 |
| PC アプリから LAN に HTTPS で待ち受け、スマホからつなげるか | 未 |

2026-10-24 に、サーバーにエージェントを置く構成をやめました。
その構成のために試した次の2つは取りやめ、記録はこのファイルの 75514d4 の版に残しています。

- エージェントを入れ替えても、シェルが切れずに残るか。unit に `KillMode=process` を書けば残った
- エージェントの転送から起動までにかかる時間。ローカルのコンテナで 0.4 秒前後だった

IP アドレスの証明書の取得は、手を付ける前に取りやめました。

下の2つの試験では、表示と入力を試すために、エージェントの試作 (`spike/cmd/drag`) をサーバー側の中継に使っています。

## 検証の環境

検証用のサーバーは Docker のコンテナで、Ubuntu 24.04、systemd 255、OpenSSH 9.6p1 です。
systemd を PID 1 で動かしています。

```
cd dev/testserver && docker compose up -d --build
```

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
docker compose exec -d -u asuta -e HOME=/home/asuta -e SHELL=/bin/bash -e LANG=C.UTF-8 -e DRAG_LISTEN=0.0.0.0:8080 testserver drag agent
cd spike/phonetest && npm install && node phone.mjs
```

Chrome DevTools Protocol の `Input.synthesizeScrollGesture` では、素の横スクロールの要素でも動きませんでした。
試験では `Input.dispatchTouchEvent` でタッチを1つずつ送っています。

分かっていないことが2つあります。

- iPhone の Safari と Android の Chrome の実機では試していません。Chrome のスマホの表示で試しただけです
- 再送は直近の 512 KiB だけなので、全画面のアプリを開いてからそれ以上の出力があると、代替画面に切り替えた出力が再送から外れます。そのときの表示は試していません

## Wails の上の xterm.js で、IME の変換と確定が通る

`spike/pcapp` は Wails v2.17.0 の vanilla のひな形から作った試作で、SSH でつないでエージェントのセッションを xterm.js 6.0.0 に表示します。
WebView2 は 155.0.4283.62 です。
この PC の既定の IME の Meltype で、`echo ` に続けて にほんご と打ち、変換して確定し、実行しました。

- 変換中の文字はカーソルの位置に出て、候補の窓もその下に出た
- 確定した 日本語 はシェルの行に入り、カーソルは2桁幅の文字の後ろに来た
- 実行すると 日本語 と出力された

キーは SendInput の仮想キーで送り、IME の開け閉めは WM_IME_CONTROL で行いました。
Unicode の文字として送るツールでは IME を通らず、英字のまま入りました。
Microsoft IME と Google 日本語入力では、まだ試していません。
