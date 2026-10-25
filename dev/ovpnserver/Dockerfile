# DRAG の OpenVPN の試験用のサーバー。ローカルでだけ使う
# 設定の違う 7 つのサーバーを動かし、VPN の中の :22 を検証用の SSH サーバー (dev/testserver) へ渡す
FROM alpine:3.22
RUN apk add --no-cache openvpn openssl socat
COPY entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh
CMD ["/entrypoint.sh"]
