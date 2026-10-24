package remote

import (
	"bufio"
	"cmp"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"
)

// リソース監視の詳細。どれも開いている間だけ数秒おきに読む

// Proc はプロセス1つ。CPU は前に読んだときからの使用率 (1コアを使い切って 100%)
type Proc struct {
	PID  int     `json:"pid"`
	User string  `json:"user"`
	Name string  `json:"name"`
	Cmd  string  `json:"cmd"`
	CPU  float64 `json:"cpu"`
	RSS  int64   `json:"rss"`
	// Container は、このプロセスが動いている Docker のコンテナの名前。名前を読めなければ ID の頭 12 文字
	Container string `json:"container"`
}

const procScript = `ps -eo pid=,user:32=,rss=,args= 2>/dev/null
echo __TICKS__
for d in /proc/[0-9]*; do read -r l < $d/stat 2>/dev/null && echo "$l"; done
echo __HZ__
getconf CLK_TCK 2>/dev/null || echo 100
echo __CG__
grep -o -H "docker[-/][0-9a-f]\{64\}" /proc/[0-9]*/cgroup 2>/dev/null || true`

type procSnap struct {
	at    time.Time
	ticks map[int]uint64
}

// Processes はプロセスの一覧。CPU の使用率は、前に呼んだときからの CPU 時間の増え方で出す。初めて呼んだときは 0
func (s *Server) Processes() ([]Proc, error) {
	out, err := s.outputPlain("exec /bin/sh -c '" + procScript + "'")
	if err != nil {
		return nil, err
	}
	now := time.Now()
	head, rest, _ := strings.Cut(out, "__TICKS__\n")
	stats, rest, _ := strings.Cut(rest, "__HZ__\n")
	hzs, cgs, _ := strings.Cut(rest, "__CG__\n")
	hz, _ := strconv.ParseFloat(strings.TrimSpace(hzs), 64)
	if hz <= 0 {
		hz = 100
	}
	ticks := map[int]uint64{}
	names := map[int]string{}
	for _, l := range strings.Split(stats, "\n") {
		// "pid (comm) state ppid ..." の形。comm には空白や括弧が入りうるので、最後の ) で分ける
		open, close := strings.IndexByte(l, '('), strings.LastIndexByte(l, ')')
		if open < 0 || close < open {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(l[:open]))
		if err != nil {
			continue
		}
		f := strings.Fields(l[close+1:])
		if len(f) < 13 {
			continue
		}
		u, _ := strconv.ParseUint(f[11], 10, 64)
		st, _ := strconv.ParseUint(f[12], 10, 64)
		ticks[pid] = u + st
		names[pid] = l[open+1 : close]
	}

	s.mu.Lock()
	prev := s.procPrev
	s.procPrev = procSnap{at: now, ticks: ticks}
	s.mu.Unlock()
	dt := now.Sub(prev.at).Seconds()

	list := []Proc{}
	for _, l := range strings.Split(head, "\n") {
		f := strings.Fields(l)
		if len(f) < 4 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		rss, _ := strconv.ParseInt(f[2], 10, 64)
		p := Proc{PID: pid, User: f[1], RSS: rss * 1024, Cmd: strings.Join(f[3:], " "), Name: names[pid]}
		if p.Name == "" {
			p.Name = f[3]
		}
		if old, ok := prev.ticks[pid]; ok && dt > 0 && ticks[pid] >= old {
			p.CPU = float64(ticks[pid]-old) / hz / dt * 100
		}
		list = append(list, p)
	}
	if ids := containerIDs(cgs); len(ids) > 0 {
		names := s.containerNames(ids)
		for i := range list {
			if id := ids[list[i].PID]; id != "" {
				list[i].Container = cmp.Or(names[id], id[:12])
			}
		}
	}
	return list, nil
}

// containerIDs は、grep で拾った cgroup のパス (/proc/<番号>/cgroup:docker-<ID>) から、
// プロセスの番号とコンテナの ID の表を作る。cgroup v2 の systemd では docker-<ID>.scope、
// cgroupfs や v1 では /docker/<ID> の形でパスに入る
func containerIDs(out string) map[int]string {
	ids := map[int]string{}
	for _, l := range strings.Split(out, "\n") {
		file, m, ok := strings.Cut(strings.TrimSpace(l), ":")
		if !ok || len(m) < 64 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(file, "/proc/"), "/cgroup"))
		if err != nil {
			continue
		}
		ids[pid] = m[len(m)-64:]
	}
	return ids
}

// containerNames は、コンテナの ID から名前を引く表。知らない ID があれば docker ps で読み直す。
// docker を使えなければ空
func (s *Server) containerNames(ids map[int]string) map[string]string {
	s.mu.Lock()
	names := s.cnames
	s.mu.Unlock()
	fresh := true
	for _, id := range ids {
		if _, ok := names[id]; !ok {
			fresh = false
			break
		}
	}
	if fresh {
		return names
	}
	cmd, stdin := s.command(`exec docker ps --no-trunc --format "{{.ID}} {{.Names}}"`, "")
	out, err := s.combined(cmd, stdin)
	if err != nil {
		return names
	}
	names = map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if id, name, ok := strings.Cut(strings.TrimSpace(l), " "); ok {
			names[id] = name
		}
	}
	s.mu.Lock()
	s.cnames = names
	s.mu.Unlock()
	return names
}

// Mount はファイルシステム1つ
type Mount struct {
	Path string `json:"path"`
	Dev  string `json:"dev"`
	Size int64  `json:"size"`
	Used int64  `json:"used"`
}

// Mounts は、ディスクのファイルシステム。tmpfs やシステムの仮のものは外す
func (s *Server) Mounts() ([]Mount, error) {
	out, err := s.outputPlain("exec /bin/sh -c 'df -P -k 2>/dev/null'")
	if err != nil && out == "" {
		return nil, err
	}
	list := []Mount{}
	for _, l := range strings.Split(out, "\n")[1:] {
		f := strings.Fields(l)
		if len(f) < 6 {
			continue
		}
		dev, mp := f[0], strings.Join(f[5:], " ")
		if slices.Contains([]string{"tmpfs", "devtmpfs", "shm", "udev", "none", "efivarfs", "overlay"}, dev) && mp != "/" {
			continue
		}
		if mp != "/" && slices.ContainsFunc([]string{"/dev", "/run", "/sys", "/proc", "/snap", "/boot/efi"}, func(p string) bool { return mp == p || strings.HasPrefix(mp, p+"/") }) {
			continue
		}
		size, _ := strconv.ParseInt(f[1], 10, 64)
		used, _ := strconv.ParseInt(f[2], 10, 64)
		list = append(list, Mount{Path: mp, Dev: dev, Size: size * 1024, Used: used * 1024})
	}
	return list, nil
}

// Usage はフォルダの中のファイルの大きさの合計と数
type Usage struct {
	Size  int64 `json:"size"`
	Files int   `json:"files"`
}

// DirUsage はフォルダの中のファイルを数える。読めないところは数えない
func (s *Server) DirUsage(dir string) (Usage, error) {
	cmd, stdin := s.command(`IFS= read -r d; find "$d" -type f -printf "%s\n" 2>/dev/null | awk "{s+=\$1; n++} END {print s+0, n+0}"`, dir+"\n")
	out, err := s.combined(cmd, stdin)
	if err != nil {
		return Usage{}, errors.New(strings.TrimSpace(out))
	}
	var u Usage
	if _, err := fmt.Sscan(strings.TrimSpace(out), &u.Size, &u.Files); err != nil {
		return Usage{}, fmt.Errorf("find を読めない: %q", out)
	}
	return u, nil
}

// DirSize はフォルダ1つの大きさ
type DirSize struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// BigDirs は、dir の下の2段目までのフォルダを大きい順に返す。sudo モードなら読めないフォルダも数える
func (s *Server) BigDirs(dir string, n int) ([]DirSize, error) {
	cmd, stdin := s.command(`IFS= read -r d; du -x -k -d 2 -- "$d" 2>/dev/null; exit 0`, dir+"\n")
	sess, err := s.ssh.NewSession()
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	sess.Stdin = strings.NewReader(stdin)
	out, err := sess.Output(cmd)
	if err != nil {
		return nil, err
	}
	list := []DirSize{}
	for _, l := range strings.Split(string(out), "\n") {
		size, p, ok := strings.Cut(l, "\t")
		if !ok || p == dir {
			continue
		}
		k, err := strconv.ParseInt(size, 10, 64)
		if err != nil {
			continue
		}
		list = append(list, DirSize{Path: p, Size: k * 1024})
	}
	slices.SortFunc(list, func(a, b DirSize) int { return cmp.Compare(b.Size, a.Size) })
	return list[:min(n, len(list))], nil
}

// NetInfo はネットワークのインターフェース・接続・待ち受け
type NetInfo struct {
	Ifaces []Iface  `json:"ifaces"`
	Conns  []Conn   `json:"conns"`
	Listen []Listen `json:"listen"`
}

type Iface struct {
	Name  string   `json:"name"`
	Addrs []string `json:"addrs"`
}

// Conn は TCP の接続1つ。Proc は分かったときだけ。ほかの利用者のプロセスは sudo モードでないと分からない
type Conn struct {
	Remote string `json:"remote"`
	Port   int    `json:"port"` // こちら側のポート
	Proc   string `json:"proc"`
}

type Listen struct {
	Port int    `json:"port"`
	Addr string `json:"addr"`
	Proc string `json:"proc"`
}

// ip が無いサーバーもあるので、TCP の表は /proc/net から読み、プロセスはソケットの inode から引く
const netScript = `for f in /proc/net/tcp /proc/net/tcp6; do [ -r $f ] && while read -r l; do echo "T $l"; done < $f; done
while read -r l; do echo "I $l"; done < /proc/net/dev
command -v ip >/dev/null 2>&1 && ip -o addr show 2>/dev/null | while read -r n name fam addr rest; do echo "A $name $addr"; done
for d in /proc/[0-9]*; do read -r c < $d/comm 2>/dev/null && echo "C ${d#/proc/} $c"; done
ls -l /proc/[0-9]*/fd 2>/dev/null
exit 0`

func (s *Server) Network() (NetInfo, error) {
	cmd, stdin := s.command(netScript, "")
	sess, err := s.ssh.NewSession()
	if err != nil {
		return NetInfo{}, err
	}
	defer sess.Close()
	sess.Stdin = strings.NewReader(stdin)
	out, err := sess.Output(cmd)
	if err != nil {
		return NetInfo{}, err
	}
	return parseNet(string(out)), nil
}

func parseNet(out string) NetInfo {
	info := NetInfo{Ifaces: []Iface{}, Conns: []Conn{}, Listen: []Listen{}}
	comm := map[string]string{}
	owner := map[string]string{} // inode → プロセス名
	type row struct{ local, remote, state, inode string }
	var rows []row
	addrs := map[string][]string{}
	var ifaces []string
	pid := ""
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		l := sc.Text()
		switch {
		case strings.HasPrefix(l, "T "):
			f := strings.Fields(l[2:])
			if len(f) >= 10 && f[0] != "sl" {
				rows = append(rows, row{f[1], f[2], f[3], f[9]})
			}
		case strings.HasPrefix(l, "I "):
			name, _, ok := strings.Cut(l[2:], ":")
			name = strings.TrimSpace(name)
			if ok && name != "lo" && !strings.Contains(name, "|") {
				ifaces = append(ifaces, name)
			}
		case strings.HasPrefix(l, "A "):
			f := strings.Fields(l[2:])
			if len(f) == 2 {
				addrs[f[0]] = append(addrs[f[0]], f[1])
			}
		case strings.HasPrefix(l, "C "):
			p, c, _ := strings.Cut(l[2:], " ")
			comm[p] = c
		case strings.HasPrefix(l, "/proc/") && strings.HasSuffix(l, "/fd:"):
			pid = strings.TrimSuffix(strings.TrimPrefix(l, "/proc/"), "/fd:")
		case strings.Contains(l, "socket:["):
			i := strings.Index(l, "socket:[")
			inode := strings.TrimSuffix(l[i+len("socket:["):], "]")
			if pid != "" {
				owner[inode] = comm[pid]
			}
		}
	}
	for _, name := range ifaces {
		info.Ifaces = append(info.Ifaces, Iface{Name: name, Addrs: append([]string{}, addrs[name]...)})
	}
	seen := map[string]bool{}
	for _, r := range rows {
		lip, lport := hexAddr(r.local)
		switch r.state {
		case "0A": // LISTEN。IPv4 と IPv6 の両方で待ち受けていても1つにする
			key := strconv.Itoa(lport)
			if seen[key] {
				continue
			}
			seen[key] = true
			info.Listen = append(info.Listen, Listen{Port: lport, Addr: lip, Proc: owner[r.inode]})
		case "01": // ESTABLISHED
			rip, rport := hexAddr(r.remote)
			info.Conns = append(info.Conns, Conn{Remote: net.JoinHostPort(rip, strconv.Itoa(rport)), Port: lport, Proc: owner[r.inode]})
		}
	}
	slices.SortFunc(info.Listen, func(a, b Listen) int { return a.Port - b.Port })
	return info
}

// hexAddr は /proc/net/tcp の "0100007F:1F90" を 127.0.0.1 と 8080 にする。
// アドレスは 4 バイトごとにホストのバイト順 (x86 ではリトルエンディアン) で書かれている
func hexAddr(s string) (string, int) {
	a, p, ok := strings.Cut(s, ":")
	if !ok {
		return s, 0
	}
	port, _ := strconv.ParseUint(p, 16, 16)
	b, err := hex.DecodeString(a)
	if err != nil || (len(b) != 4 && len(b) != 16) {
		return a, int(port)
	}
	for i := 0; i < len(b); i += 4 {
		b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
	}
	ip := net.IP(b)
	if v4 := ip.To4(); v4 != nil {
		return v4.String(), int(port)
	}
	return ip.String(), int(port)
}
