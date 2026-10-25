# tailnet の中の端末1つ。Headscale にログインし、届いた 22 番を検証用の SSH サーバーへ渡す
FROM alpine:3.22
RUN apk add --no-cache tailscale socat
COPY entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh
CMD ["/entrypoint.sh"]
