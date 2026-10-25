#!/bin/sh
# accel-ppp の SSTP を動かす。ユーザー名 asuta、パスワード drag (MS-CHAPv2)
#   この PC には 192.168.40.0/24 から割り当て、サーバーは 192.168.40.1
#   サーバーの証明書 (名前は vpn.test と 127.0.0.1) を作り、その CA を /out/ca.crt に書く
#   VPN の中の 192.168.40.1:22 を検証用の SSH サーバー (dev/testserver) へ渡す
set -e
mkdir -p /out /etc/accel
[ -e /dev/ppp ] || mknod /dev/ppp c 108 0
cd /etc/accel
openssl req -x509 -newkey rsa:2048 -nodes -keyout ca.key -days 3650 -subj /CN=drag-accel-ca -out ca.crt 2>/dev/null
openssl req -newkey rsa:2048 -nodes -keyout server.key -subj /CN=vpn.test -out server.csr 2>/dev/null
cat > server.ext <<'EOF'
basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=DNS:vpn.test,IP:127.0.0.1
EOF
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 3650 -extfile server.ext -out server.crt 2>/dev/null
cat server.crt server.key > server.pem
cp ca.crt /out/ca.crt
echo 'asuta * drag *' > chap-secrets

cat > accel-ppp.conf <<'EOF'
[modules]
log_file
sstp
auth_mschap_v2
chap-secrets
ippool

[core]
log-error=/dev/stderr
thread-count=2

[log]
log-file=/dev/stdout
level=5

[ppp]
verbose=1
ipv4=require
ipv6=deny
mtu=1400
mru=1400
lcp-echo-interval=30

[auth]
any-login=0

[sstp]
port=443
accept=ssl
ssl-pemfile=/etc/accel/server.pem
verbose=1

[chap-secrets]
chap-secrets=/etc/accel/chap-secrets
gw-ip-address=192.168.40.1

[ip-pool]
gw-ip-address=192.168.40.1
192.168.40.10-50

# つないでよいクライアントの外のアドレス
[client-ip-range]
0.0.0.0/0
EOF

socat TCP-LISTEN:22,fork,reuseaddr TCP:host.docker.internal:2222 &
exec accel-pppd -c /etc/accel/accel-ppp.conf
