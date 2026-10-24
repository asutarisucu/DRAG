package store

import (
	"testing"
	"time"
)

// 1秒ごとの値を、分ごとと時間ごとの平均にまとめて、ファイルに残す
func TestHistoryAveragesAndPersists(t *testing.T) {
	dir := t.TempDir()
	h := OpenHistory(dir, "asuta@example:22")
	start := time.Date(2026, 10, 25, 9, 58, 0, 0, time.UTC)
	// 9:58 は CPU 10、9:59 は CPU 30、10:00 に入ったところで 9:59 までが分と時間に残る
	for i := 0; i < 60; i++ {
		h.Add(start.Add(time.Duration(i)*time.Second), Point{CPU: 10, Rx: 100})
	}
	for i := 0; i < 60; i++ {
		h.Add(start.Add(time.Minute+time.Duration(i)*time.Second), Point{CPU: 30, Rx: 300})
	}
	h.Add(start.Add(2*time.Minute), Point{CPU: 50})
	h.Add(start.Add(3*time.Minute), Point{CPU: 50})

	min := h.Points("minute")
	if len(min) != 3 || min[0].CPU != 10 || min[1].CPU != 30 || min[1].Rx != 300 || min[0].T != start.UnixMilli() {
		t.Fatalf("%+v", min)
	}
	hr := h.Points("hour")
	if len(hr) != 1 || hr[0].CPU != 20 || hr[0].T != start.Truncate(time.Hour).UnixMilli() {
		t.Fatalf("%+v", hr)
	}

	again := OpenHistory(dir, "asuta@example:22")
	if len(again.Points("minute")) != 3 || len(again.Points("hour")) != 1 {
		t.Fatal("読み直すと消えている")
	}
}
