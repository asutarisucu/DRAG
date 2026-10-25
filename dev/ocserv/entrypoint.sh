#!/bin/sh
# ocserv を動かす。ユーザー名 asuta、パスワード drag。グループは dev と ops の2つから選ばせる
#   この PC には 192.168.50.0/24 から割り当て、DNS は 192.168.50.1
#   サーバーの証明書 (名前は vpn.test と 127.0.0.1) を作り、その CA を /out/ca.crt に書く
set -e
mkdir -p /out /etc/ocserv
cd /etc/ocserv
openssl req -x509 -newkey rsa:2048 -nodes -keyout ca.key -days 3650 -subj /CN=drag-ocserv-ca -out ca.crt 2>/dev/null
openssl req -newkey rsa:2048 -nodes -keyout server.key -subj /CN=vpn.test -out server.csr 2>/dev/null
cat > server.ext <<'EOF'
basicConstraints=CA:FALSE
keyUsage=digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=DNS:vpn.test,IP:127.0.0.1
EOF
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 3650 -extfile server.ext -out server.crt 2>/dev/null
cp ca.crt /out/ca.crt
expect -c 'spawn ocpasswd -c /etc/ocserv/ocpasswd -g "dev,ops" asuta; expect "password:"; send "drag\r"; expect "password:"; send "drag\r"; expect eof' > /dev/null

cat > ocserv.conf <<'EOF'
auth = "plain[passwd=/etc/ocserv/ocpasswd]"
tcp-port = 443
socket-file = /run/ocserv-socket
server-cert = /etc/ocserv/server.crt
server-key = /etc/ocserv/server.key
max-clients = 16
max-same-clients = 4
keepalive = 30
dpd = 60
try-mtu-discovery = false
tls-priorities = "NORMAL:%SERVER_PRECEDENCE"
auth-timeout = 240
cookie-timeout = 300
deny-roaming = false
rekey-time = 172800
rekey-method = ssl
use-occtl = false
pid-file = /run/ocserv.pid
device = vpns
predictable-ips = true
ipv4-network = 192.168.50.0/24
dns = 192.168.50.1
route = 192.168.50.0/24
select-group = dev
select-group = ops
cisco-client-compat = true
EOF

socat TCP-LISTEN:22,fork,reuseaddr TCP:host.docker.internal:2222 &
exec ocserv -c /etc/ocserv/ocserv.conf -f
