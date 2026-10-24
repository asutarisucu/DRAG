package main

import (
	"embed"

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
		OnStartup:   app.startup,
		OnShutdown:  app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
