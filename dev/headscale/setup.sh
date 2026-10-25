#!/bin/sh
# Headscale にユーザー drag を作り、使い回せるログインの鍵と API の鍵を out/ に書く
#   out/authkey  端末をログインさせる鍵 (peer と試験が使う)
#   out/apikey   試験がブラウザのログインの代わりに端末を登録するのに使う
# 試験は app のフォルダで次のように動かす
#   GOOS=linux CGO_ENABLED=0 go test -c -o ../dev/headscale/out/tsvpn.test ./internal/vpn/tsvpn
#   docker compose run --rm tester /out/tsvpn.test -test.v
set -e
cd "$(dirname "$0")"
mkdir -p out
hs() { docker compose exec -T headscale headscale "$@"; }
until hs users list >/dev/null 2>&1; do sleep 1; done
user() { hs users list -o json | tr -d ' \t\r\n' | sed -n 's/.*"id":\([0-9]*\),"name":"drag".*/\1/p'; }
id=$(user)
[ -n "$id" ] || { hs users create drag >/dev/null; id=$(user); }
hs preauthkeys create --user "$id" --reusable --expiration 24h | tail -1 > out/authkey
hs apikeys create --expiration 24h | tail -1 > out/apikey
echo "user drag ($id): out/authkey と out/apikey を書きました"
