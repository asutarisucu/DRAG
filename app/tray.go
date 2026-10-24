package main

import (
	_ "embed"
	"os"
	goruntime "runtime"
	"sync"

	"fyne.io/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 通知領域のアイコン。窓を閉じても PC アプリは SSH の接続とスマホの待ち受けを続け、
// ここから窓を開き直すか終了する

//go:embed build/windows/icon.ico
var trayIcon []byte

var trayLabels = map[string][2]string{
	"ja": {"DRAG を開く", "終了"},
	"en": {"Open DRAG", "Quit"},
}

type tray struct {
	mu         sync.Mutex
	lang       string
	open, quit *systray.MenuItem
}

// startTray は通知領域にアイコンを置く。DRAG_NO_TRAY があれば置かない。試すときに、作り直すたびにアイコンが増えないように
func (a *App) startTray() {
	if os.Getenv("DRAG_NO_TRAY") != "" {
		return
	}
	a.tray = &tray{lang: "ja"}
	go func() {
		// Windows の窓のメッセージは、窓を作ったスレッドでしか受け取れない。作るのと受け取るのを同じスレッドで行う
		goruntime.LockOSThread()
		systray.Run(func() {
			systray.SetIcon(trayIcon)
			systray.SetTooltip("DRAG")
			systray.SetOnTapped(a.showWindow)
			a.tray.mu.Lock()
			l := trayLabels[a.tray.lang]
			a.tray.open = systray.AddMenuItem(l[0], "")
			systray.AddSeparator()
			a.tray.quit = systray.AddMenuItem(l[1], "")
			open, quit := a.tray.open, a.tray.quit
			a.tray.mu.Unlock()
			for {
				select {
				case <-open.ClickedCh:
					a.showWindow()
				case <-quit.ClickedCh:
					runtime.Quit(a.ctx)
					return
				}
			}
		}, nil)
	}()
}

func (a *App) stopTray() {
	if a.tray != nil {
		systray.Quit()
	}
}

func (a *App) showWindow() {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

// SetLanguage は、通知領域のメニューを画面の言語に合わせる
func (a *App) SetLanguage(lang string) {
	if a.tray == nil {
		return
	}
	l, ok := trayLabels[lang]
	if !ok {
		return
	}
	a.tray.mu.Lock()
	defer a.tray.mu.Unlock()
	a.tray.lang = lang
	if a.tray.open != nil {
		a.tray.open.SetTitle(l[0])
		a.tray.quit.SetTitle(l[1])
	}
}
