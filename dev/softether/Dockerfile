# DRAG の SSTP (と L2TP/IPsec) の試験用のサーバー (SoftEther VPN)。ローカルでだけ使う
# SecureNAT で VPN の中にアドレスを配り、VPN の外へ NAT で出す。
# 入れ物の中の :22 を検証用の SSH サーバー (dev/testserver) へ渡す
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends build-essential curl ca-certificates socat iproute2 openssl \
 && rm -rf /var/lib/apt/lists/*
RUN curl -sL https://github.com/SoftEtherVPN/SoftEtherVPN_Stable/releases/download/v4.44-9807-rtm/softether-vpnserver-v4.44-9807-rtm-2025.04.16-linux-x64-64bit.tar.gz \
  | tar xz -C /opt && make -C /opt/vpnserver
COPY entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh
CMD ["/entrypoint.sh"]
