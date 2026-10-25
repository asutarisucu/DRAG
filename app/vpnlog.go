package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/asutarisucu/DRAG/app/internal/vpn"
)

// vpnLog は、VPN を張るときと通すときの様子を設定フォルダの vpn.log に書く。うまくつながらないときに読む。
// 1 MB を超えたら vpn.old.log に移して書き直す
type vpnLog struct {
	mu   sync.Mutex
	path string
}

func startVPNLog() {
	l := &vpnLog{path: filepath.Join(configDir(), "vpn.log")}
	vpn.Logf = l.printf
}

func (l *vpnLog) printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if fi, err := os.Stat(l.path); err == nil && fi.Size() > 1<<20 {
		os.Rename(l.path, filepath.Join(filepath.Dir(l.path), "vpn.old.log"))
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, args...))
}
