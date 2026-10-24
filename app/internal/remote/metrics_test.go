package remote

import (
	"bufio"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func sampleOutput(user, idle, rx, sectors uint64) string {
	return strings.Join([]string{
		"S cpu " + itoa(user) + " 0 0 " + itoa(idle) + " 0 0 0 0 0 0",
		"S cpu0 " + itoa(user) + " 0 0 " + itoa(idle) + " 0 0 0 0 0 0",
		"M MemTotal: 1000",
		"M MemAvailable: 400",
		"M Cached: 100",
		"M SwapTotal: 50",
		"M SwapFree: 20",
		"N   lo: 999 0 0 0 0 0 0 0 999 0 0 0 0 0 0 0",
		"N eth0: " + itoa(rx) + " 0 0 0 0 0 0 0 " + itoa(rx/2) + " 0 0 0 0 0 0 0",
		"D 259 0 nvme0n1 0 0 " + itoa(sectors) + " 0 0 0 " + itoa(sectors) + " 0 0 0 0",
		"D 259 1 nvme0n1p1 0 0 999999 0 0 0 999999 0 0 0 0",
		"U 3600.5",
		"L 0.50 0.40 0.30 1/100 42",
		"F 2000 500",
		sampleEnd,
	}, "\n") + "\n"
}

func itoa(n uint64) string { return strconv.FormatUint(n, 10) }

// 2回分の値の差から、使用率と速さを出す
func TestSamplerComputesRates(t *testing.T) {
	sp := &sampler{prevCPU: map[string]cpuTimes{}}
	out := bufio.NewReader(strings.NewReader(sampleOutput(100, 900, 1000, 0) + sampleOutput(130, 970, 3000, 8)))
	if _, err := sp.read(io.Discard, out); err != nil {
		t.Fatal(err)
	}
	sp.prevAt = sp.prevAt.Add(-time.Second)
	m, err := sp.read(io.Discard, out)
	if err != nil {
		t.Fatal(err)
	}
	// 100 刻みのうち 30 が使用
	if math.Abs(m.CPU-30) > 0.01 || len(m.Cores) != 1 || math.Abs(m.Cores[0]-30) > 0.01 {
		t.Fatalf("CPU %v %v", m.CPU, m.Cores)
	}
	if m.MemTotal != 1000*1024 || m.MemUsed != 600*1024 || m.SwapUsed != 30*1024 {
		t.Fatalf("メモリ %+v", m)
	}
	// 1 秒の間に eth0 で 2000 バイト受信。lo は数えない。パーティションは数えない
	if math.Abs(m.Rx-2000) > 50 || math.Abs(m.Tx-1000) > 25 || math.Abs(m.DiskRead-8*512) > 150 {
		t.Fatalf("速さ rx %v tx %v read %v", m.Rx, m.Tx, m.DiskRead)
	}
	if m.Uptime != 3600.5 || len(m.Load) != 3 || m.Load[0] != 0.5 || m.DiskTotal != 2000*1024 || m.DiskUsed != 500*1024 {
		t.Fatalf("%+v", m)
	}
}

func TestWholeDisk(t *testing.T) {
	for name, want := range map[string]bool{
		"sda": true, "sda1": false, "vdb": true, "nvme0n1": true, "nvme0n1p2": false,
		"mmcblk0": true, "mmcblk0p1": false, "loop3": false, "ram0": false, "dm-0": false,
	} {
		if wholeDisk(name) != want {
			t.Errorf("%s", name)
		}
	}
}

// 検証用のサーバーから、1秒ごとに値が届く
func TestStartMetrics(t *testing.T) {
	s := dialTrusted(t)
	got := make(chan Metrics, 4)
	stop, err := s.StartMetrics(func(m Metrics) { got <- m })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	select {
	case m := <-got:
		if m.MemTotal <= 0 || len(m.Cores) == 0 || m.Uptime <= 0 || m.DiskTotal <= 0 {
			t.Fatalf("%+v", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("値が届かない")
	}
}

func TestHexAddr(t *testing.T) {
	for in, want := range map[string]string{
		"0100007F:1F90":                         "127.0.0.1:8080",
		"00000000:0016":                         "0.0.0.0:22",
		"00000000000000000000000001000000:0050": "::1:80",
	} {
		ip, port := hexAddr(in)
		if got := ip + ":" + strconv.Itoa(port); got != want {
			t.Errorf("%s → %s", in, got)
		}
	}
}

func TestParseNet(t *testing.T) {
	out := strings.Join([]string{
		"T sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode",
		"T 0: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 111 1 0 100 0 0 10 0",
		"T 1: 0200A8C0:0016 0500A8C0:D431 01 00000000:00000000 00:00000000 00000000     0        0 222 1 0 100 0 0 10 0",
		"I Inter-|   Receive",
		"I  face |bytes",
		"I     lo: 1 2",
		"I   eth0: 3 4",
		"A eth0 192.168.0.2/24",
		"C 42 sshd",
		"/proc/42/fd:",
		"lrwx------ 1 root root 64 Oct 25 01:00 3 -> socket:[111]",
		"lrwx------ 1 root root 64 Oct 25 01:00 4 -> socket:[222]",
	}, "\n")
	n := parseNet(out)
	if len(n.Ifaces) != 1 || n.Ifaces[0].Name != "eth0" || len(n.Ifaces[0].Addrs) != 1 {
		t.Fatalf("%+v", n.Ifaces)
	}
	if len(n.Listen) != 1 || n.Listen[0].Port != 22 || n.Listen[0].Proc != "sshd" {
		t.Fatalf("%+v", n.Listen)
	}
	if len(n.Conns) != 1 || n.Conns[0].Remote != "192.168.0.5:54321" || n.Conns[0].Port != 22 || n.Conns[0].Proc != "sshd" {
		t.Fatalf("%+v", n.Conns)
	}
}

// 検証用のサーバーで、プロセス・ファイルシステム・大きいフォルダ・ネットワークを読める
func TestDetails(t *testing.T) {
	s := dialTrusted(t)
	ps, err := s.Processes()
	if err != nil || !slices.ContainsFunc(ps, func(p Proc) bool { return p.Name == "sshd" && p.RSS > 0 }) {
		t.Fatalf("%v %d", err, len(ps))
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := s.Processes(); err != nil {
		t.Fatal(err)
	}
	ms, err := s.Mounts()
	if err != nil || !slices.ContainsFunc(ms, func(m Mount) bool { return m.Path == "/" && m.Size > 0 }) {
		t.Fatalf("%+v %v", ms, err)
	}
	home, _ := s.Home()
	if _, err := s.BigDirs(home, 5); err != nil {
		t.Fatal(err)
	}
	n, err := s.Network()
	if err != nil || !slices.ContainsFunc(n.Listen, func(l Listen) bool { return l.Port == 22 }) || len(n.Conns) == 0 {
		t.Fatalf("%+v %v", n, err)
	}
}
