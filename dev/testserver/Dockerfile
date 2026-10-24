# DRAG の検証用の SSH サーバー。ローカルでだけ使う
# systemd を PID 1 で動かし、サービスの操作と sudo モードを確かめる
FROM ubuntu:24.04
RUN apt-get update \
 && apt-get install -y --no-install-recommends systemd systemd-sysv dbus openssh-server screen tmux vim-tiny sudo procps ca-certificates zsh fish \
 && rm -rf /var/lib/apt/lists/* \
 && systemctl mask getty@tty1.service systemd-udevd.service \
 && systemctl enable ssh.service
ARG USER=asuta
ARG PASS=drag
RUN useradd -m -s /bin/bash "$USER" && echo "$USER:$PASS" | chpasswd && usermod -aG sudo "$USER"
# ログインシェルが bash でない利用者。前面のコマンドを調べる検証で使う
# zsh は .zshrc が無いと初回の設定の画面を出すので、空のものを置く
RUN useradd -m -s /usr/bin/zsh zuser && echo "zuser:$PASS" | chpasswd && touch /home/zuser/.zshrc && chown zuser: /home/zuser/.zshrc \
 && useradd -m -s /usr/bin/fish fuser && echo "fuser:$PASS" | chpasswd
EXPOSE 22
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
