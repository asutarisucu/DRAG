package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/asutarisucu/DRAG/app/internal/remote"
	"github.com/asutarisucu/DRAG/app/internal/store"
)

// リソース監視。接続している間、使用量を1秒ごとに読んで "metrics" (番号, 値) で送り、PC に記録する

// startMetrics は s の使用量を読み始める。接続が切れると読み取りも終わる。Linux でなければ何もしない
func (a *App) startMetrics(id string, c *conn, s *remote.Server) {
	if c.hist == nil {
		port := c.req.Port
		if port == 0 {
			port = 22
		}
		c.hist = store.OpenHistory(configDir(), fmt.Sprintf("%s@%s_%d", c.req.User, c.req.Host, port))
	}
	hist := c.hist
	s.StartMetrics(func(m remote.Metrics) {
		a.emit("metrics", id, m)
		hist.Add(time.UnixMilli(m.Time), store.Point{CPU: m.CPU, Mem: pct(m.MemUsed, m.MemTotal), Disk: pct(m.DiskUsed, m.DiskTotal), Rx: m.Rx, Tx: m.Tx})
	})
}

func pct(used, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return 100 * float64(used) / float64(total)
}

// History は使用量の記録。span は "minute" (24時間分) か "hour" (30日分)
func (a *App) History(serverID, span string) ([]store.Point, error) {
	a.mu.Lock()
	c := a.servers[serverID]
	a.mu.Unlock()
	if c == nil || c.hist == nil {
		return []store.Point{}, nil
	}
	return c.hist.Points(span), nil
}

func (a *App) Processes(serverID string) ([]remote.Proc, error) {
	s, err := a.server(serverID)
	if err != nil {
		return nil, err
	}
	return s.Processes()
}

func (a *App) Mounts(serverID string) ([]remote.Mount, error) {
	s, err := a.server(serverID)
	if err != nil {
		return nil, err
	}
	return s.Mounts()
}

// BigDirs は、dir の下の大きいフォルダ。dir が空なら、ディスクのファイルシステムごとに最上位から数える。
// NFS (host:/path) と CIFS (//host/share) は、数えるとネットワークの先を全部読むので外す
func (a *App) BigDirs(serverID, dir string) (remote.DiskDirs, error) {
	s, err := a.server(serverID)
	if err != nil {
		return remote.DiskDirs{}, err
	}
	if dir != "" {
		return s.BigDirs([]string{dir}, 12)
	}
	ms, err := s.Mounts()
	if err != nil {
		return remote.DiskDirs{}, err
	}
	roots := []string{}
	for _, m := range ms {
		if !strings.Contains(m.Dev, ":") && !strings.HasPrefix(m.Dev, "//") {
			roots = append(roots, m.Path)
		}
	}
	return s.BigDirs(roots, 12)
}

func (a *App) Network(serverID string) (remote.NetInfo, error) {
	s, err := a.server(serverID)
	if err != nil {
		return remote.NetInfo{}, err
	}
	return s.Network()
}
