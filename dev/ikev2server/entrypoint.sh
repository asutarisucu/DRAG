#!/bin/sh
# CA とサーバーの証明書を作り、strongSwan を動かす。クライアントが使う CA の証明書を /out/ca.crt に書く
#   ユーザー名 asuta、パスワード drag の EAP-MSCHAPv2。サーバーは証明書 (名前 vpn.test と 127.0.0.1) で名乗る
#   この PC には 10.10.10.0/24 から割り当て、DNS は 10.20.0.1
set -e
mkdir -p /out /etc/swanctl/x509ca /etc/swanctl/x509 /etc/swanctl/private
cd /tmp
openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out ca.key 2>/dev/null
openssl req -x509 -new -key ca.key -days 3650 -subj /CN=drag-ikev2-ca -out ca.crt
openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out server.key 2>/dev/null
openssl req -new -key server.key -subj /CN=vpn.test -out server.csr
printf 'basicConstraints=CA:FALSE\nkeyUsage=digitalSignature\nextendedKeyUsage=serverAuth,1.3.6.1.5.5.8.2.2\nsubjectAltName=DNS:vpn.test,IP:127.0.0.1\n' > server.ext
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -days 3650 -extfile server.ext -out server.crt 2>/dev/null
cp ca.crt /etc/swanctl/x509ca/ && cp ca.crt /out/ca.crt
cp server.crt /etc/swanctl/x509/ && cp server.key /etc/swanctl/private/

cat > /etc/swanctl/swanctl.conf <<'EOF'
connections {
  rw {
    version = 2
    pools = rw
    # 鍵の取り替えを確かめるため、IKE の SA は 50 秒ごとに替える
    rekey_time = 50s
    local {
      auth = pubkey
      certs = server.crt
      id = vpn.test
    }
    remote {
      auth = eap-mschapv2
      eap_id = %any
    }
    children {
      net {
        local_ts = 0.0.0.0/0
        # 鍵の取り替えを確かめるため、子の SA は 40 秒ごとに替える
        rekey_time = 40s
      }
    }
  }
}
pools {
  rw {
    addrs = 10.10.10.0/24
    dns = 10.20.0.1
  }
}
secrets {
  eap-asuta {
    id = asuta
    secret = drag
  }
}
EOF

# VPN の中の 10.20.0.1:22 を、検証用の SSH サーバーへ渡す
ip addr add 10.20.0.1/32 dev lo
socat TCP-LISTEN:22,fork,reuseaddr TCP:host.docker.internal:2222 &
/usr/lib/strongswan/charon &
sleep 2
swanctl --load-all
wait
