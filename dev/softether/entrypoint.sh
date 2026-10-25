#!/bin/sh
# SoftEther VPN を設定して動かす。
#   ハブ DEFAULT に、ユーザー名 asuta、パスワード drag。SecureNAT で 192.168.30.0/24 を配る
#   SSTP は 443 番。サーバーの証明書 (名前は vpn.test と 127.0.0.1) を作り、その CA を /out/ca.crt に書く
#   VPN の中から SSH でつなぐ先 (この入れ物のアドレス) を /out/target に書く
set -e
mkdir -p /out
cd /opt/vpnserver
./vpnserver start
sleep 3
cmd() { ./vpncmd localhost /SERVER /CMD "$@" > /dev/null; }
hub() { ./vpncmd localhost /SERVER /HUB:DEFAULT /CMD "$@" > /dev/null; }
hub UserCreate asuta /GROUP:none /REALNAME:none /NOTE:none
hub UserPasswordSet asuta /PASSWORD:drag
hub SecureNatEnable
cmd SstpEnable yes
cmd IPsecEnable /L2TP:yes /L2TPRAW:no /ETHERIP:no /PSK:dragpsk /DEFAULTHUB:DEFAULT
cd /tmp
openssl req -x509 -newkey rsa:2048 -nodes -keyout ca.key -days 3650 -subj /CN=drag-sstp-ca -out ca.crt 2>/dev/null
openssl req -newkey rsa:2048 -nodes -keyout server.key -subj /CN=vpn.test -out server.csr 2>/dev/null
printf 'basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=DNS:vpn.test,IP:127.0.0.1
' > server.ext
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 3650 -extfile server.ext -out server.crt 2>/dev/null
cp ca.crt /out/ca.crt
cd /opt/vpnserver
cmd ServerCertSet /LOADCERT:/tmp/server.crt /LOADKEY:/tmp/server.key
hostname -i | awk '{print $1}' > /out/target

socat TCP-LISTEN:22,fork,reuseaddr TCP:host.docker.internal:2222 &
echo ready
tail -f /dev/null
