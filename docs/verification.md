# DRAG 技術検証の結果

企画書の7章に挙げた検証の結果を、終わった順に残します。
試作のコードは `spike/` に、検証用のサーバーは `dev/testserver/` にあります。

| 検証 | 状態 |
|---|---|
| Wails の WebView2 の上で、xterm.js に日本語入力が通るか | Meltype では通る。Microsoft IME と Google 日本語入力は未確認 (2026-10-24) |
| PC の幅のターミナルを、スマホで横スクロールさせて表示できるか | できる。縦のスクロールは自前の処理が要る。Android の Chrome の実機でも動いた。iPhone は未確認 (2026-10-24) |
| 前面で動いているコマンドを、SSH の別のチャネルから調べて切り出せるか | できる。bash・zsh・fish で、打ってから 0.25 秒前後で分かる (2026-10-24) |
| sudo モードで、ファイルの操作とコマンドを sudo の権限で行えるか | できる。パスワードが違うときと NOPASSWD のときに手当てが要る (2026-10-24) |
| PC アプリから LAN に HTTPS で待ち受け、スマホからつなげるか | Android の Chrome の実機から、証明書の警告から進んでつながる。日本語の入力も通る。iPhone は未確認 (2026-10-24) |
| IP アドレスの証明書を、PC アプリから取得して更新し続けられるか | 未 |

2026-10-24 に、サーバーにエージェントを置く構成をやめました。
その構成のために試した次の2つは取りやめ、記録はこのファイルの 75514d4 の版に残しています。

- エージェントを入れ替えても、シェルが切れずに残るか。unit に `KillMode=process` を書けば残った
- エージェントの転送から起動までにかかる時間。ローカルのコンテナで 0.4 秒前後だった

スマホの横スクロールと Wails の日本語入力の試験では、表示と入力を試すために、エージェントの試作 (`spike/cmd/drag`) をサーバー側の中継に使っています。

## 検証の環境

検証用のサーバーは Docker のコンテナで、Ubuntu 24.04、systemd 255、OpenSSH 9.6p1 です。
systemd を PID 1 で動かしています。
ログインシェルが bash の asuta のほかに、zsh の zuser と fish の fuser がいます。パスワードはどれも drag です。

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

- iPhone の Safari の実機では試していません。Android の Chrome の実機では、LAN の待ち受けの検証で、同じ処理を入れたページの横と縦のスクロールが動きました
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

## 前面のコマンドは、SSH の別のチャネルで /proc を読めば分かる

`spike/cmd/fgwatch` が、PC アプリと同じく SSH のチャネルだけを使って次を確かめ、全部通りました。

```
cd spike && go run ./cmd/fgwatch
```

シェルの PID は、シェルを開くときに知ります。
PTY 付きの exec のチャネルで次を実行し、出てきた1行は画面に出す前に取り除きます。

```
exec /bin/sh -c 'IFS= read -r d; cd "${d:-$HOME}"; echo "DRAG-<乱数> $$"; exec "$SHELL" -l'
```

sh の PID のまま、ログインシェルに exec します。
開くフォルダは、引用符の扱いがシェルごとに違うので、コマンドに埋めずに標準入力の1行目で渡します。
ログインシェルが bash・zsh・fish のどれでも PID が分かり、開いてから 15〜41 ms でコマンドを受け付けました。

前面のコマンドは、PTY を付けない別のチャネルで sh を1つ開いておき、PC アプリから1行ずつコマンドを送って調べます。
`read -r l < /proc/<PID>/stat` で8番目の項目の、端末の前面にいるプロセスグループの番号を読みます。
シェルの PID と違えば、`/proc/<番号>/cmdline` を読みます。
read と echo は sh の組み込みなので、調べるたびにサーバーでプロセスが増えることはありません。
1回の往復は、ローカルのコンテナで 0.6 ms 前後でした。
0.25 秒おきに調べると、打ってから 0.25 秒前後で分かりました。

| 打ったもの | 前面のコマンド |
|---|---|
| `sleep 2` | `sleep 2`。終わるとシェルに戻る |
| `sleep 30 \| cat` | `sleep 30`。パイプでは先頭のコマンドになる |
| `vi /tmp/fg.txt` | `vi /tmp/fg.txt` |
| `screen -S fg`、`tmux new -s fg` | screen と tmux。デタッチするとシェルに戻り、セッションは残る |
| `ssh asuta@localhost` | ssh。パスワードを入れる前から分かる |
| `bash` | bash |
| `sudo -s` | `sudo -s` |

切り出しは、前面のコマンドが動いているチャネルをそのままウィンドウに移し、ターミナルには新しいシェルを開く形で試しました。
`readlink /proc/<PID>/cwd` で作業中のフォルダを読み、そのフォルダで新しいシェルを開くと、pwd が返るまで 15 ms でした。
元のチャネルでは sleep 30 が動いたままでした。

試していないことが3つあります。

- /proc を hidepid 付きでマウントしたサーバー。sudo で動く root のプロセスが見えなくなる見込み
- Linux 以外のサーバー。/proc の形が違う
- `sudo docker exec` のように sudo を前に付けたコマンド。前面のコマンドは `sudo -s` と同じく sudo から始まるので、自動で移すコマンドと比べるときは先頭の sudo を飛ばす (案)

## sudo モードは、パスワードを exec のチャネルの標準入力で sudo に渡せば成り立つ

`spike/cmd/sudoprobe` で次を確かめ、全部通りました。

```
cd spike && go run ./cmd/sudoprobe
```

コマンドは `sudo -k -S -p '' <コマンド>` で実行し、標準入力の1行目にパスワードを書きます。
`-k` を付けて、前に通った認証を使わないようにしています。
前の認証を使うと sudo はパスワードを読まず、パスワードの行がコマンドの入力に入ります。

| 試したこと | 結果 |
|---|---|
| ON にするときに `sudo -k -S -p '' -v` でパスワードを確かめる | 正しいパスワードで 30 ms 前後。違うパスワードでは 2 秒前後たってから失敗が返る |
| `id -un` を sudo で実行する | root |
| sudo で起動した sftp-server と SFTP で話す | 始めるまで 37 ms。/root の一覧、/etc/shadow の読み出し、/etc へのファイルの作成・名前の変更・権限の変更・削除ができた |
| 利用者のファイルを、sudo の SFTP で書き換える | 持ち主は利用者のまま |
| unit を SFTP で置き、systemctl で起動して止める | できた |
| プロセスの一覧を見る | パスワードは出ない |
| sudo のログを見る | 実行したコマンドが journal に残る |

sftp-server の場所は、`/etc/ssh/sshd_config` の Subsystem の行から取りました。
Ubuntu では `/usr/lib/openssh/sftp-server` です。

SFTP では、つまずいたところが3つありました。

1つ目は、exec のチャネルを開始する前に標準入力へパスワードを書くと、sudo がパスワードを待ったまま止まったことです。
開始したあとに書くと通りました。

2つ目は、パスワードが違うと止まることです。
sudo は違うパスワードを受けると、次の行をパスワードとして読みにいきます。
SFTP の最初のパケットには改行が入らないので、sudo は読み続け、PC アプリは SFTP の返事を待ち続けました。
そこで sudo で `sh -c 'echo DRAG-SUDO-OK; exec <sftp-server>'` を起動し、その1行が出てから SFTP を始めるようにしました。
プロンプトは `-p DRAG-PW:` にして、標準エラーにプロンプトが2回出たら、パスワードが違うとみなしてチャネルを閉じます。
この形で、違うパスワードは 2.7 秒で失敗と分かり、サーバーに sudo は残りませんでした。
sudo のログには、sftp-server ではなく sh -c の行が残ります。

3つ目は、NOPASSWD の設定があるときです。
sudo はパスワードを読まないので、パスワードの行が sftp-server に入って SFTP が崩れました。
ON にするときに `sudo -n true` が通れば、パスワードを送らずに `sudo -n` で起動します。

試していないことが2つあります。

- sudoers に requiretty を書いたサーバー。端末のないチャネルでは sudo が断る見込み
- Subsystem に internal-sftp を書いたサーバー。sftp-server の場所が書かれていないので、よく置かれる場所を順に探す (案)

## LAN の待ち受けには、Android の Chrome の実機から、証明書の警告から進んでつながる

`spike/cmd/lanserve` は PC アプリの代わりに HTTPS で待ち受け、スマホのブラウザに検証用サーバーのシェルを出します。
起動のたびに自己署名の証明書を作ります。
ECDSA P-256 の鍵で、SAN に PC の IPv4 アドレスを入れ、用途を serverAuth、有効期間を 397 日にしました。
iOS 13 以降は、SAN と serverAuth のない証明書と、有効期間が 825 日を超える証明書では TLS の接続が失敗するためです ([Apple のサポート記事](https://support.apple.com/en-us/103769))。

端末の登録の代わりに、起動のたびに作る合言葉を URL に付けます。
合言葉の URL を開くと、合言葉を cookie (HttpOnly、Secure、SameSite=Strict) に移します。
WebSocket は、cookie の合言葉と接続元 (Origin) が合うときだけ開きます。

PC の上で 127.0.0.1 に待ち受け、Chrome のスマホの表示から `spike/phonetest/lan.mjs` で試して、全部通りました。

```
cd spike && go build -o lanserve.exe ./cmd/lanserve && ./lanserve.exe -listen 127.0.0.1:8443
cd spike/phonetest && node lan.mjs "https://127.0.0.1:8443/?t=<合言葉>"
```

| 試したこと | 結果 |
|---|---|
| 合言葉なし、違う合言葉で開く | 403 |
| 合言葉の URL を開く | cookie に移り、本体のページに移る |
| 別の接続元からと、cookie なしで WebSocket を開く | 403 |
| ページの WebSocket | 50 ms 前後でつながり、シェルの出力が届く。打ったコマンドが動く |
| 127.0.0.1 ではない名前で開く | 証明書の警告が出る。警告から進むと、isSecureContext が true で、crypto.subtle、navigator.clipboard、localStorage が使え、WebSocket もつながる |

127.0.0.1 は、証明書がどうであれ安全なページとして扱われます。
スマホと同じ状態にするため、Chrome の `--host-resolver-rules` で drag.test という名前を 127.0.0.1 に向け、警告の画面から進みました。

この PC には、LAN のアドレスのほかに、169.254 で始まるリンクローカルのアドレスが7つと、WSL の仮想のアダプターのアドレスがありました。
QR コードに出すアドレスは、既定の経路を持つアダプターのものを選ぶ必要があります (案)。

続けて、LAN のすべてのアドレスに待ち受け、Android のスマホの Chrome 152 から試しました。
スマホは PC と同じ Wi-Fi につなぎ、PC の画面に出した QR コードを読んで開きました。

```
cd spike && go build -o lanserve.exe ./cmd/lanserve && ./lanserve.exe -listen :8443
```

| 試したこと | 結果 |
|---|---|
| PC で待ち受けを始める | Windows のファイアウォールの確認が出た。許可すると、lanserve.exe の受信を許可するルールが TCP と UDP の2つ、パブリックのプロファイルにできた。この PC の Wi-Fi はパブリックになっていた |
| QR コードを読んで開く | 証明書の警告が出た。警告から進むまでの間、PC の側には `tls: unknown certificate` が出た |
| 警告から進む | isSecureContext が true で、crypto.subtle、navigator.clipboard、localStorage が使えた。WebSocket は 128 ms でつながり、134 ms でシェルの出力が届いた |
| `touch /tmp/from-phone` と打つ | ファイルができた |
| スマホのキーボードで 日本語 と変換し、`echo 日本語 > /tmp/ja.txt` で書き出す | 日本語 がそのまま入った。文字の重なりや抜けはない |
| 指でターミナルを横と縦に動かす | 目で見て、どちらも動くことを確かめた |

ファイアウォールのルールは実行ファイルのパスごとにできます。
パブリックで許可すると、家の外の Wi-Fi でも受け付けます。
家の Wi-Fi をプライベートに変えてからプライベートだけを許可するよう、初回に案内する形が考えられます (案)。

試していないのは iPhone の Safari です。
警告を受け入れたあとに WebSocket がつながるかを確かめたいです。
