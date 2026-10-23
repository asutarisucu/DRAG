# DRAG の検証用の SSH サーバー。ローカルでだけ使う
FROM ubuntu:24.04
RUN apt-get update \
 && apt-get install -y --no-install-recommends openssh-server screen tmux sudo procps ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && mkdir -p /run/sshd
ARG USER=asuta
ARG PASS=drag
RUN useradd -m -s /bin/bash "$USER" && echo "$USER:$PASS" | chpasswd && usermod -aG sudo "$USER"
EXPOSE 22
CMD ["/usr/sbin/sshd", "-D", "-e"]
