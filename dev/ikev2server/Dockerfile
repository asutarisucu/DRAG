# DRAG の IKEv2 の試験用のサーバー (strongSwan)。ローカルでだけ使う
# VPN の中の 10.20.0.1:22 を検証用の SSH サーバー (dev/testserver) へ渡す
FROM alpine:3.22
RUN apk add --no-cache strongswan openssl socat iproute2
COPY entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh
CMD ["/entrypoint.sh"]
