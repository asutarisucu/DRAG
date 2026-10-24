#!/bin/sh
# 検証用の偽の docker。DRAG の Docker のウィンドウを、本物の Docker なしで試すために置く。
# 出力の形は docker/cli@v27.5.1 の --format '{{json .}}' に合わせる。状態は /var/lib/fakedocker に置く。
# root か docker のグループの利用者だけが使える。ほかは本物と同じく permission denied で終わる
S=/var/lib/fakedocker
umask 002
deny() {
  echo 'permission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock: Get "http://%2Fvar%2Frun%2Fdocker.sock/v1.47/containers/json": dial unix /var/run/docker.sock: connect: permission denied' >&2
  exit 1
}
[ "$(id -u)" = 0 ] || id -nG | tr ' ' '\n' | grep -qx docker || deny

# 状態の行は 名前|イメージ|状態|ポート|Compose のプロジェクト|Compose のファイル|マウント (種類:元:先 を ; で区切る)
[ -f $S/containers ] || {
  mkdir -p $S
  cat > $S/containers <<'EOF'
web|nginx:1.27|running|0.0.0.0:8080->80/tcp|app|/home/asuta/app/compose.yaml|volume:web-data:/usr/share/nginx/html
redis|redis:7|running||app|/home/asuta/app/compose.yaml|volume:redis-data:/data
backup|alpine:3.20|exited||||bind:/home/asuta/backups:/backups
EOF
  printf 'nginx|1.27|188MB\nredis|7|117MB\nalpine|3.20|7.8MB\nhello-world|latest|13.3kB\n' > $S/images
  printf 'web-data\nredis-data\nold-data\n' > $S/volumes
}

# フラグを読み飛ばし、-- の次か最後の引数を対象にする
target() {
  t=""
  while [ $# -gt 0 ]; do
    case $1 in --) shift; t=$1; break;; -*) ;; *) t=$1;; esac
    shift
  done
  echo "$t"
}
has() { grep -q "^$1|" $S/containers; }
nosuch() { echo "Error response from daemon: No such container: $1" >&2; exit 1; }

cmd=$1; shift
case $cmd in
ps)
  while IFS='|' read -r n img st ports proj comp mounts; do
    labels=""
    [ -n "$proj" ] && labels="com.docker.compose.project=$proj,com.docker.compose.project.config_files=$comp"
    names=$(echo "$mounts" | tr ';' '\n' | cut -d: -f2 | paste -sd, -)
    if [ "$st" = running ]; then status="Up 3 days"; else status="Exited (0) 2 hours ago"; fi
    id=$(echo "$n" | sha256sum | cut -c1-64)
    printf '{"ID":"%s","Names":"%s","Image":"%s","State":"%s","Status":"%s","Ports":"%s","Labels":"%s","Mounts":"%s"}\n' "$id" "$n" "$img" "$st" "$status" "$ports" "$labels" "$names"
  done < $S/containers;;
images)
  while IFS='|' read -r r tag size; do
    printf '{"ID":"sha256:%s","Repository":"%s","Tag":"%s","Size":"%s","CreatedSince":"2 weeks ago"}\n' "$(echo "$r:$tag" | sha256sum | cut -c1-12)" "$r" "$tag" "$size"
  done < $S/images;;
volume)
  sub=$1; shift
  case $sub in
  ls) while read -r v; do printf '{"Driver":"local","Name":"%s","Mountpoint":"/var/lib/docker/volumes/%s/_data"}\n' "$v" "$v"; done < $S/volumes;;
  rm)
    v=$(target "$@")
    grep -qx "$v" $S/volumes || { echo "Error response from daemon: get $v: no such volume" >&2; exit 1; }
    grep -q "volume:$v:" $S/containers && { echo "Error response from daemon: remove $v: volume is in use" >&2; exit 1; }
    grep -vx "$v" $S/volumes > $S/v.tmp; mv $S/v.tmp $S/volumes; echo "$v";;
  esac;;
start|stop|restart)
  n=$(target "$@"); has "$n" || nosuch "$n"
  new=running; [ $cmd = stop ] && new=exited
  awk -F'|' -v OFS='|' -v n="$n" -v s="$new" '$1==n{$3=s}1' $S/containers > $S/c.tmp && mv $S/c.tmp $S/containers
  echo "$n";;
rm)
  n=$(target "$@"); has "$n" || nosuch "$n"
  grep -q "^$n|[^|]*|running|" $S/containers && { echo "Error response from daemon: cannot remove container \"/$n\": container is running: stop the container before removing or force remove" >&2; exit 1; }
  grep -v "^$n|" $S/containers > $S/c.tmp; mv $S/c.tmp $S/containers; echo "$n";;
rmi)
  i=$(target "$@")
  r=${i%:*}; tag=${i##*:}; [ "$r" = "$i" ] && tag=latest
  grep -q "^$r|$tag|" $S/images || { echo "Error response from daemon: No such image: $i" >&2; exit 1; }
  grep -q "^[^|]*|$r:$tag|" $S/containers && { echo "Error response from daemon: conflict: unable to remove repository reference \"$i\" (must force) - container is using its referenced image" >&2; exit 1; }
  grep -v "^$r|$tag|" $S/images > $S/i.tmp; mv $S/i.tmp $S/images; echo "Untagged: $i";;
pull)
  i=$(target "$@"); r=${i%:*}; tag=${i##*:}; [ "$r" = "$i" ] && tag=latest
  grep -q "^$r|$tag|" $S/images || echo "$r|$tag|5.2MB" >> $S/images
  echo "Status: Downloaded newer image for $r:$tag";;
inspect)
  n=$(target "$@"); has "$n" || nosuch "$n"
  grep "^$n|" $S/containers | cut -d'|' -f7 | tr ';' '\n' | awk -F: 'BEGIN{printf "["} NF==3{if(c++)printf ","; if($1=="volume")printf "{\"Type\":\"volume\",\"Name\":\"%s\",\"Source\":\"/var/lib/docker/volumes/%s/_data\",\"Destination\":\"%s\",\"Driver\":\"local\",\"Mode\":\"z\",\"RW\":true,\"Propagation\":\"\"}",$2,$2,$3; else printf "{\"Type\":\"bind\",\"Source\":\"%s\",\"Destination\":\"%s\",\"Mode\":\"\",\"RW\":true,\"Propagation\":\"rprivate\"}",$2,$3} END{print "]"}';;
stats)
  grep '|running|' $S/containers | while IFS='|' read -r n rest; do printf '{"Name":"%s","CPUPerc":"0.%s%%","MemUsage":"%s.1MiB / 31.2GiB"}\n' "$n" "$(date +%S)" "${#n}"; done;;
logs)
  n=$(target "$@"); has "$n" || nosuch "$n"
  i=1
  while [ $i -le 5 ]; do echo "$(date +%H:%M:%S) $n log line $i"; i=$((i+1)); done
  case " $* " in *" -f "*) while sleep 1; do echo "$(date +%H:%M:%S) $n log line $i"; i=$((i+1)); done;; esac;;
system)
  # DRAG が呼ぶ docker system df -v --format "{{range .Volumes}}{{.Name}} {{.Size}}{{println}}{{end}}" の出力の形だけを返す
  i=1; while read -r v; do echo "$v ${i}2.5MB"; i=$((i+1)); done < $S/volumes;;
exec)
  # docker exec -it 名前 コマンド の形なので、フラグでない最初の引数が名前
  n=""
  for a in "$@"; do case $a in -*) ;; *) n=$a; break;; esac; done
  has "$n" || nosuch "$n"
  echo "fake container $n"
  exec /bin/sh -i;;
*)
  echo "fake docker: $cmd は使えません" >&2; exit 1;;
esac
