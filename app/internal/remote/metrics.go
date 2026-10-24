package remote

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Metrics はある時点のサーバーの使用量。量はバイト、速さはバイト毎秒
type Metrics struct {
	Time      int64     `json:"time"` // Unix ミリ秒
	CPU       float64   `json:"cpu"`  // 全体の使用率 (%)
	Cores     []float64 `json:"cores"`
	MemTotal  int64     `json:"memTotal"`
	MemUsed   int64     `json:"memUsed"`
	MemCache  int64     `json:"memCache"`
	SwapTotal int64     `json:"swapTotal"`
	SwapUsed  int64     `json:"swapUsed"`
	Rx        float64   `json:"rx"` // 受信
	Tx        float64   `json:"tx"` // 送信
	DiskRead  float64   `json:"diskRead"`
	DiskWrite float64   `json:"diskWrite"`
	DiskUsed  int64     `json:"diskUsed"` // / のファイルシステム
	DiskTotal int64     `json:"diskTotal"`
	Uptime    float64   `json:"uptime"` // 秒
	Load      []float64 `json:"load"`
}

const sampleEnd = "__DRAG_SAMPLE_END__"

// 1回分の読み取り。/proc は組み込みの read で読むので、サーバーでプロセスは増えない。
// df だけはプロセスを起こすので、10回に1回にする
const sampleScript = `while read -r a b; do case $a in cpu*) echo "S $a $b";; *) break;; esac; done < /proc/stat
while read -r a b c; do case $a in MemTotal:|MemAvailable:|Cached:|Buffers:|SReclaimable:|SwapTotal:|SwapFree:) echo "M $a $b";; esac; done < /proc/meminfo
while read -r l; do echo "N $l"; done < /proc/net/dev
while read -r l; do echo "D $l"; done < /proc/diskstats
read -r u x < /proc/uptime; echo "U $u"
read -r l < /proc/loadavg; echo "L $l"
`

type cpuTimes struct{ idle, total uint64 }

// sampler は、サーバーの使用量を1秒ごとに読んで fn に渡す。前の値との差から使用率と速さを出す
type sampler struct {
	prevCPU  map[string]cpuTimes
	prevNet  [2]uint64
	prevDisk [2]uint64
	prevAt   time.Time
	n        int
	diskUsed int64
	diskSize int64
}

// StartMetrics は、使用量を1秒ごとに読んで fn に渡す。返した関数で止める。Linux 以外では読めないので err を返す
func (s *Server) StartMetrics(fn func(Metrics)) (func(), error) {
	sess, err := s.ssh.NewSession()
	if err != nil {
		return nil, err
	}
	in, _ := sess.StdinPipe()
	out, _ := sess.StdoutPipe()
	if err := sess.Start("exec /bin/sh"); err != nil {
		sess.Close()
		return nil, err
	}
	br := bufio.NewReader(out)
	sp := &sampler{prevCPU: map[string]cpuTimes{}}
	// 最初の1回で /proc が読めるかを確かめる
	if _, err := sp.read(in, br); err != nil {
		sess.Close()
		return nil, err
	}
	var once sync.Once
	stop := make(chan struct{})
	go func() {
		defer sess.Close()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
			}
			m, err := sp.read(in, br)
			if err != nil {
				return
			}
			fn(m)
		}
	}()
	return func() { once.Do(func() { close(stop) }) }, nil
}

func (sp *sampler) read(in io.Writer, br *bufio.Reader) (Metrics, error) {
	script := sampleScript
	if sp.n%10 == 0 {
		script += "df -P -k / 2>/dev/null | while read -r f sz us av p mp; do echo \"F $sz $us\"; done\n"
	}
	sp.n++
	if _, err := io.WriteString(in, script+"echo "+sampleEnd+"\n"); err != nil {
		return Metrics{}, err
	}
	now := time.Now()
	m := Metrics{Time: now.UnixMilli()}
	cpu := map[string]cpuTimes{}
	var cores []string
	var rx, tx, rd, wr uint64
	mem := map[string]int64{}
	got := false
	for {
		l, err := br.ReadString('\n')
		if err != nil {
			return Metrics{}, err
		}
		l = strings.TrimRight(l, "\n")
		if l == sampleEnd {
			break
		}
		kind, rest, _ := strings.Cut(l, " ")
		f := strings.Fields(rest)
		switch kind {
		case "S":
			if len(f) < 5 {
				continue
			}
			var t cpuTimes
			for i, v := range f[1:] {
				n, _ := strconv.ParseUint(v, 10, 64)
				// idle と iowait を休みとして数える。guest は user に含まれているので足さない
				if i >= 8 {
					break
				}
				t.total += n
				if i == 3 || i == 4 {
					t.idle += n
				}
			}
			cpu[f[0]] = t
			if f[0] != "cpu" {
				cores = append(cores, f[0])
			}
			got = true
		case "M":
			if len(f) >= 2 {
				n, _ := strconv.ParseInt(f[1], 10, 64)
				mem[strings.TrimSuffix(f[0], ":")] = n * 1024
			}
		case "N":
			// "eth0: 123 ..." の形。lo は数えない
			name, vals, ok := strings.Cut(rest, ":")
			if !ok {
				continue
			}
			name = strings.TrimSpace(name)
			v := strings.Fields(vals)
			if name == "lo" || len(v) < 9 {
				continue
			}
			r, _ := strconv.ParseUint(v[0], 10, 64)
			t, _ := strconv.ParseUint(v[8], 10, 64)
			rx += r
			tx += t
		case "D":
			// 物理のディスクだけ。パーティションや loop、ram は数えない
			if len(f) < 10 || !wholeDisk(f[2]) {
				continue
			}
			r, _ := strconv.ParseUint(f[5], 10, 64)
			w, _ := strconv.ParseUint(f[9], 10, 64)
			rd += r * 512
			wr += w * 512
		case "U":
			m.Uptime, _ = strconv.ParseFloat(rest, 64)
		case "L":
			for _, v := range f[:min(3, len(f))] {
				x, _ := strconv.ParseFloat(v, 64)
				m.Load = append(m.Load, x)
			}
		case "F":
			if len(f) == 2 {
				sz, err1 := strconv.ParseInt(f[0], 10, 64)
				us, err2 := strconv.ParseInt(f[1], 10, 64)
				if err1 == nil && err2 == nil {
					sp.diskSize, sp.diskUsed = sz*1024, us*1024
				}
			}
		}
	}
	if !got {
		return Metrics{}, fmt.Errorf("/proc/stat を読めません")
	}
	usage := func(name string) float64 {
		cur, prev := cpu[name], sp.prevCPU[name]
		dt := cur.total - prev.total
		if prev.total == 0 || dt == 0 || cur.total < prev.total {
			return 0
		}
		return 100 * float64(dt-(cur.idle-prev.idle)) / float64(dt)
	}
	m.CPU = usage("cpu")
	for _, c := range cores {
		m.Cores = append(m.Cores, usage(c))
	}
	sp.prevCPU = cpu

	m.MemTotal = mem["MemTotal"]
	if avail, ok := mem["MemAvailable"]; ok {
		m.MemUsed = m.MemTotal - avail
	}
	m.MemCache = mem["Cached"] + mem["Buffers"] + mem["SReclaimable"]
	m.SwapTotal = mem["SwapTotal"]
	m.SwapUsed = mem["SwapTotal"] - mem["SwapFree"]

	if !sp.prevAt.IsZero() {
		dt := now.Sub(sp.prevAt).Seconds()
		rate := func(cur, prev uint64) float64 {
			if cur < prev || dt <= 0 {
				return 0
			}
			return float64(cur-prev) / dt
		}
		m.Rx, m.Tx = rate(rx, sp.prevNet[0]), rate(tx, sp.prevNet[1])
		m.DiskRead, m.DiskWrite = rate(rd, sp.prevDisk[0]), rate(wr, sp.prevDisk[1])
	}
	sp.prevNet, sp.prevDisk, sp.prevAt = [2]uint64{rx, tx}, [2]uint64{rd, wr}, now
	m.DiskUsed, m.DiskTotal = sp.diskUsed, sp.diskSize
	return m, nil
}

// wholeDisk は、diskstats の名前がパーティションでない物理のディスクか
func wholeDisk(name string) bool {
	for _, p := range []string{"loop", "ram", "dm-", "zram", "sr", "fd"} {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	last := name[len(name)-1]
	switch {
	case strings.HasPrefix(name, "nvme"), strings.HasPrefix(name, "mmcblk"):
		// nvme0n1 はディスク、nvme0n1p1 はパーティション
		return !strings.Contains(name[strings.LastIndexAny(name, "n")+1:], "p")
	case last >= '0' && last <= '9':
		return false
	}
	return true
}
