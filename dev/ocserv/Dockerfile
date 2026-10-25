# DRAG の AnyConnect 互換の試験用のサーバー (ocserv)。ローカルでだけ使う
# VPN の中の 192.168.50.1:22 を検証用の SSH サーバー (dev/testserver) へ渡す
FROM debian:bookworm-slim
RUN apt-get update  && apt-get install -y --no-install-recommends ocserv openssl socat iproute2 expect  && rm -rf /var/lib/apt/lists/*
COPY entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh
CMD ["/entrypoint.sh"]
