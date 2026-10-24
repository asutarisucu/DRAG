package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "DRAG",
		Width:     1440,
		Height:    900,
		MinWidth:  900,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// 画面の --bg と同じ色。読み込むまでの間に白く光らないように
		BackgroundColour: &options.RGBA{R: 12, G: 15, B: 19, A: 1},
		// PC のファイルをファイルツリーへドロップしてアップロードする
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true},
		// 窓を閉じても通知領域に残り、SSH の接続とスマホの待ち受けを続ける。終了は通知領域のメニューから
		HideWindowOnClose: true,
		// 画面をブラウザから試すときに、作り直すたびに窓が前に出ないようにする
		StartHidden: os.Getenv("DRAG_START_HIDDEN") != "",
		// 通知領域に残っている間にもう一度起動したら、新しく起動せずに今の窓を出す
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               instanceID,
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.showWindow() },
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
