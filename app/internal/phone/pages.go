package phone

import (
	_ "embed"
	"encoding/base64"
	"html/template"
	"net/http"
	"strings"
)

// 画面を出す前にスマホに見せるページ。言語の設定はまだ読めないので、ブラウザの言語で選ぶ
type pageKind int

const (
	pageNotRegistered pageKind = iota
	pageExpired
)

var pageText = map[pageKind]map[string][2]string{
	pageNotRegistered: {
		"ja": {"この端末は登録されていません", "PC の DRAG で「スマホを登録」を開き、出てくる QR コードをこの端末で読んでください。"},
		"en": {"This device is not registered", "Open \"Add phone\" in DRAG on your PC and scan the QR code with this device."},
	},
	pageExpired: {
		"ja": {"この QR コードは使えません", "期限が切れたか、もう使われています。PC の DRAG で QR コードを作り直して、読み直してください。"},
		"en": {"This QR code cannot be used", "It has expired or was already used. Make a new QR code in DRAG on your PC and scan it again."},
	},
}

// ロゴは登録していない端末にも出すので、画面のファイルを読ませずにページに埋め込む
//
//go:embed logo.png
var logoPNG []byte

var logoURL = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(logoPNG))

var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="{{.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>DRAG</title>
<style>
body { margin: 0; min-height: 100dvh; display: grid; place-items: center; background: radial-gradient(ellipse 90% 50% at 50% 0, rgba(124, 145, 182, .2), transparent 70%), #05060f; color: #c7d3ea; font: 15px/1.7 system-ui, sans-serif; }
main { box-sizing: border-box; width: min(420px, calc(100% - 32px)); padding: 28px 26px; border-radius: 16px; background: rgba(186, 214, 247, .03); box-shadow: inset 0 0 0 1px rgba(186, 215, 247, .12), inset 0 1px 1px rgba(216, 236, 248, .2), inset 0 24px 48px rgba(168, 216, 245, .06), 0 24px 32px rgba(6, 6, 14, .7); }
.logo { display: block; width: 200px; height: auto; margin-bottom: 14px; }
h1 { color: #d8ecf8; font-size: 19px; font-weight: 600; margin: 10px 0 8px; }
p { color: #9da7ba; margin: 0; }
</style>
</head>
<body><main><img class="logo" src="{{.Logo}}" alt="DRAG Runs As GUI"><h1>{{.Title}}</h1><p>{{.Body}}</p></main></body>
</html>
`))

func page(w http.ResponseWriter, r *http.Request, status int, k pageKind) {
	lang := "en"
	if strings.HasPrefix(r.Header.Get("Accept-Language"), "ja") {
		lang = "ja"
	}
	t := pageText[k][lang]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	pageTmpl.Execute(w, struct {
		Lang, Title, Body string
		Logo              template.URL
	}{lang, t[0], t[1], logoURL})
}
