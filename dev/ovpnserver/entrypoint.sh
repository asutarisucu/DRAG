#!/bin/sh
# 証明書と鍵を作り、設定の違う 4 つの OpenVPN サーバーを動かす。クライアントの .ovpn を /out に書く
#   a: UDP 1194  tls-crypt、ユーザー名とパスワードだけ (クライアント証明書なし)、AES-256-GCM、30 秒ごとに鍵を替える
#   b: TCP 1195  tls-auth (SHA256)、クライアント証明書、AES-128-GCM、comp-lzo no、net30
#   c: UDP 1196  包みなし、クライアント証明書、CHACHA20-POLY1305、compress stub-v2
#   d: UDP 1197  tls-auth (SHA1)、クライアント証明書、AES-256-CBC と HMAC-SHA256
set -e
P=/etc/openvpn/pki
mkdir -p $P /out
cd $P

ec() { openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$1" 2>/dev/null; }
ec ca.key
openssl req -x509 -new -key ca.key -days 3650 -subj /CN=drag-test-ca -out ca.crt
cert() {
  ec "$1.key"
  openssl req -new -key "$1.key" -subj "/CN=$1" -out "$1.csr"
  printf 'basicConstraints=CA:FALSE\nkeyUsage=digitalSignature\nextendedKeyUsage=%s\n' "$2" > "$1.ext"
  openssl x509 -req -in "$1.csr" -CA ca.crt -CAkey ca.key -CAcreateserial -days 3650 -extfile "$1.ext" -out "$1.crt" 2>/dev/null
}
cert server serverAuth
cert client clientAuth
openvpn --genkey secret ta.key
openvpn --genkey secret tc.key

cat > /etc/openvpn/check.sh <<'EOF'
#!/bin/sh
# via-file で渡されたファイルの 1 行目がユーザー名、2 行目がパスワード
[ "$(sed -n 1p "$1")" = asuta ] && [ "$(sed -n 2p "$1")" = drag ]
EOF
chmod 755 /etc/openvpn/check.sh

common() {
  cat <<EOF
dev $1
server $2 255.255.255.0
ca $P/ca.crt
cert $P/server.crt
key $P/server.key
dh none
keepalive 10 60
# 試験は並んで走るので、同じ名前のクライアントが同時につないでも前の接続を切らない
duplicate-cn
verb 3
EOF
}

{ common tun0 10.8.0.0; cat <<EOF
proto udp
port 1194
topology subnet
tls-crypt $P/tc.key
data-ciphers AES-256-GCM
verify-client-cert none
username-as-common-name
script-security 2
auth-user-pass-verify /etc/openvpn/check.sh via-file
push "dhcp-option DNS 10.8.0.1"
reneg-sec 30
EOF
} > a.conf

{ common tun1 10.8.1.0; cat <<EOF
proto tcp-server
port 1195
tls-auth $P/ta.key 0
auth SHA256
data-ciphers AES-128-GCM
comp-lzo no
push "comp-lzo no"
EOF
} > b.conf

{ common tun2 10.8.2.0; cat <<EOF
proto udp
port 1196
topology subnet
data-ciphers CHACHA20-POLY1305
compress stub-v2
push "compress stub-v2"
EOF
} > c.conf

{ common tun3 10.8.3.0; cat <<EOF
proto udp
port 1197
topology subnet
tls-auth $P/ta.key 0
auth SHA256
data-ciphers AES-256-CBC
EOF
} > d.conf

inline() { printf '<%s>\n' "$1"; cat "$2"; printf '</%s>\n' "$1"; }
client() {
  printf 'client\ndev tun\nproto %s\nremote 127.0.0.1 %s\nnobind\nremote-cert-tls server\n' "$1" "$2"
  inline ca ca.crt
}
{ client udp 1194; echo auth-user-pass; inline tls-crypt tc.key; } > /out/a.ovpn
{ client tcp 1195; echo 'auth SHA256'; echo 'comp-lzo no'; echo 'key-direction 1'; inline cert client.crt; inline key client.key; inline tls-auth ta.key; } > /out/b.ovpn
{ client udp 1196; echo 'compress stub-v2'; inline cert client.crt; inline key client.key; } > /out/c.ovpn
{ client udp 1197; echo 'auth SHA256'; echo 'cipher AES-256-CBC'; echo 'key-direction 1'; inline cert client.crt; inline key client.key; inline tls-auth ta.key; } > /out/d.ovpn

# VPN の中の :22 を、検証用の SSH サーバーへ渡す
socat TCP-LISTEN:22,fork,reuseaddr TCP:host.docker.internal:2222 &
for s in a b c d; do
  openvpn --config $P/$s.conf --log /var/log/openvpn-$s.log &
done
wait
