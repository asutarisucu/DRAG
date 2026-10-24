package phone

import (
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

var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="{{.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>DRAG</title>
<style>
body { margin: 0; min-height: 100dvh; display: grid; place-items: center; background: #0c0f13; color: #e6ebf2; font: 15px/1.7 system-ui, sans-serif; }
main { width: min(420px, calc(100% - 48px)); }
.brand { font-weight: 800; letter-spacing: .2em; color: #5b9dff; font-size: 13px; }
h1 { font-size: 19px; margin: 10px 0 8px; }
p { color: #9aa6b8; margin: 0; }
</style>
</head>
<body><main><div class="brand">DRAG</div><h1>{{.Title}}</h1><p>{{.Body}}</p></main></body>
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
	pageTmpl.Execute(w, struct{ Lang, Title, Body string }{lang, t[0], t[1]})
}
