# DRAG の検証用の SSH サーバー。ローカルでだけ使う
# systemd を PID 1 で動かし、エージェントをサービスとして登録したときの動きを確かめる
FROM ubuntu:24.04
RUN apt-get update \
 && apt-get install -y --no-install-recommends systemd systemd-sysv dbus openssh-server screen tmux sudo procps ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && systemctl mask getty@tty1.service systemd-udevd.service \
 && systemctl enable ssh.service
ARG USER=asuta
ARG PASS=drag
RUN useradd -m -s /bin/bash "$USER" && echo "$USER:$PASS" | chpasswd && usermod -aG sudo "$USER"
EXPOSE 22
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
