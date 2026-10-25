# DRAG の SSTP の試験用のサーバー (accel-ppp)。ローカルでだけ使う
# SoftEther と違い、Crypto Binding の Compound MAC を確かめる。Windows の SSTP サーバーと同じ厳しさで試す
# 動かす前に、Docker の VM に ppp_generic を読み込んでおく (compose.yaml の説明を見る)
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends build-essential cmake git ca-certificates libssl-dev libpcre2-dev openssl socat iproute2 \
 && rm -rf /var/lib/apt/lists/*
RUN git clone --depth 1 --branch 1.14.0 https://github.com/accel-ppp/accel-ppp /src \
 && mkdir /src/build && cd /src/build \
 && cmake -DBUILD_IPOE_DRIVER=FALSE -DBUILD_VLAN_MON_DRIVER=FALSE -DRADIUS=FALSE -DLUA=FALSE -DSHAPER=FALSE -DCMAKE_INSTALL_PREFIX=/usr -DCMAKE_BUILD_TYPE=Release .. \
 && make -j4 && make install
COPY entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh
CMD ["/entrypoint.sh"]
