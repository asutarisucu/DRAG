#!/bin/sh
# setup.sh が /out/authkey を書くのを待ってから、名前 peer で tailnet に加わる。
# tailscaled はユーザー空間で動かし、tailnet から 22 番に来た接続を、この中の socat へ渡す
set -e
tailscaled --tun=userspace-networking --state=mem: --socket=/tmp/ts.sock &
until [ -s /out/authkey ]; do sleep 1; done
tailscale --socket=/tmp/ts.sock up --login-server=http://headscale:8080 --authkey="$(cat /out/authkey)" --hostname=peer
exec socat TCP-LISTEN:22,fork,reuseaddr TCP:host.docker.internal:2222
