package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Point はある時間の使用量の平均。CPU・メモリ・ディスクは使用率 (%)、Rx と Tx はバイト毎秒
type Point struct {
	T    int64   `json:"t"` // その時間の始まり。Unix ミリ秒
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
	Disk float64 `json:"disk"`
	Rx   float64 `json:"rx"`
	Tx   float64 `json:"tx"`
}

const (
	minuteKeep = 24 * 60 // 1分ごとの値を24時間分
	hourKeep   = 30 * 24 // 1時間ごとの値を30日分
)

// History は1台のサーバーの使用量の記録。接続している間の値だけを持ち、接続していない間は空く
type History struct {
	path string

	mu     sync.Mutex
	Minute []Point `json:"minute"`
	Hour   []Point `json:"hour"`
	curMin acc
	curHr  acc
}

type acc struct {
	start time.Time
	sum   Point
	n     int
}

func (a *acc) add(p Point) {
	a.sum.CPU += p.CPU
	a.sum.Mem += p.Mem
	a.sum.Disk += p.Disk
	a.sum.Rx += p.Rx
	a.sum.Tx += p.Tx
	a.n++
}

func (a *acc) avg() Point {
	n := float64(a.n)
	return Point{T: a.start.UnixMilli(), CPU: a.sum.CPU / n, Mem: a.sum.Mem / n, Disk: a.sum.Disk / n, Rx: a.sum.Rx / n, Tx: a.sum.Tx / n}
}

// OpenHistory は dir の下に置いた key の記録を読む。key はサーバーごとに決まる名前
func OpenHistory(dir, key string) *History {
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		return r
	}, key)
	h := &History{path: filepath.Join(dir, "history", safe+".json")}
	if b, err := os.ReadFile(h.path); err == nil {
		json.Unmarshal(b, h)
	}
	return h
}

// Add は1秒ごとの値を足す。分や時間が変わったら、それまでの平均を記録に残す
func (h *History) Add(at time.Time, p Point) {
	h.mu.Lock()
	defer h.mu.Unlock()
	m := at.Truncate(time.Minute)
	if h.curMin.n > 0 && !m.Equal(h.curMin.start) {
		avg := h.curMin.avg()
		h.Minute = keep(append(h.Minute, avg), minuteKeep)
		hr := h.curMin.start.Truncate(time.Hour)
		if h.curHr.n > 0 && !hr.Equal(h.curHr.start) {
			h.Hour = keep(append(h.Hour, h.curHr.avg()), hourKeep)
			h.curHr = acc{}
		}
		if h.curHr.n == 0 {
			h.curHr.start = hr
		}
		h.curHr.add(avg)
		h.curMin = acc{}
		h.saveLocked()
	}
	if h.curMin.n == 0 {
		h.curMin.start = m
	}
	h.curMin.add(p)
}

func keep(list []Point, n int) []Point {
	if over := len(list) - n; over > 0 {
		return append([]Point{}, list[over:]...)
	}
	return list
}

// Points は span ("minute" か "hour") の記録を古い順に返す
func (h *History) Points(span string) []Point {
	h.mu.Lock()
	defer h.mu.Unlock()
	if span == "hour" {
		return append([]Point{}, h.Hour...)
	}
	return append([]Point{}, h.Minute...)
}

func (h *History) saveLocked() {
	b, err := json.Marshal(h)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		return
	}
	tmp := h.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, h.path)
	}
}
