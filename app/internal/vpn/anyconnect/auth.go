// Package anyconnect は AnyConnect 互換 (draft-mavrogiannopoulos-openconnect) のクライアント。
// HTTPS の XML でログインして cookie をもらい、CONNECT /CSCOSSLC/tunnel で開いた TLS の上で
// CSTP の枠に IP のパケットを包む。DTLS は使わず、TLS だけで流す。
// 仕様に書かれていない部分は openconnect v9.12 (auth.c、cstp.c) と ocserv 1.1.6 (src/worker-http.c) の動きに合わせた
package anyconnect

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

// userAgent は、ocserv が AnyConnect のクライアントとして扱う名前 (AnyConnect で始まる)
const userAgent = "AnyConnect-compatible DRAG"

// ErrAuth は、ユーザー名かパスワードが違うとき
var ErrAuth = errors.New("VPN のユーザー名かパスワードが違います")

// authDoc は、サーバーが返す <config-auth>
type authDoc struct {
	Type         string   `xml:"type,attr"`
	SessionToken string   `xml:"session-token"`
	Opaque       *opaque  `xml:"opaque"`
	Auth         authNode `xml:"auth"`
}

// opaque は、サーバーが次の返事にそのまま付けてほしいもの
type opaque struct {
	Attrs []xml.Attr `xml:",any,attr"`
	Inner string     `xml:",innerxml"`
}

type authNode struct {
	ID      string `xml:"id,attr"`
	Message string `xml:"message"`
	Error   string `xml:"error"`
	Form    struct {
		Action string `xml:"action,attr"`
		Inputs []struct {
			Type  string `xml:"type,attr"`
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"input"`
		Selects []struct {
			Name    string `xml:"name,attr"`
			Options []struct {
				Value    string `xml:"value,attr"`
				Selected string `xml:"selected,attr"`
				Label    string `xml:",chardata"`
			} `xml:"option"`
		} `xml:"select"`
	} `xml:"form"`
}

// elem は <name>value</name> を書く
func elem(b *strings.Builder, name, value string) {
	b.WriteString("<" + name + ">")
	xml.EscapeText(b, []byte(value))
	b.WriteString("</" + name + ">")
}

// query は、こちらから送る <config-auth> を作る
func query(typ string, o *opaque, body string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<config-auth client="vpn" type="` + typ + `" aggregate-auth-version="2">`)
	b.WriteString(`<version who="vpn">` + userAgent + `</version><device-id>win</device-id>`)
	if o != nil {
		b.WriteString("<opaque")
		for _, a := range o.Attrs {
			b.WriteString(" " + a.Name.Local + `="`)
			xml.EscapeText(&b, []byte(a.Value))
			b.WriteString(`"`)
		}
		b.WriteString(">" + o.Inner + "</opaque>")
	}
	b.WriteString(body)
	b.WriteString("</config-auth>")
	return b.String()
}

// login は、サーバーのフォームに答えてログインし、CSTP に使う cookie (webvpn) を返す
func login(ctx context.Context, cfg Config, base *url.URL, tc *tls.Config) (string, error) {
	jar, _ := cookiejar.New(nil)
	hc := &http.Client{
		Jar:       jar,
		Transport: &http.Transport{TLSClientConfig: tc},
		// 移った先にも同じ XML を送り直す
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer hc.CloseIdleConnections()

	var init strings.Builder
	elem(&init, "group-access", base.String())
	if cfg.Group != "" {
		elem(&init, "group-select", cfg.Group)
	}
	body := query("init", nil, init.String())
	at := base
	passName := "" // パスワードを入れた欄の名前
	for range 12 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, at.String(), strings.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("X-Transcend-Version", "1")
		req.Header.Set("X-Aggregate-Auth", "1")
		req.Header.Set("Content-Type", "application/xml; charset=utf-8")
		resp, err := hc.Do(req)
		if err != nil {
			return "", fmt.Errorf("VPN のサーバーと HTTPS でつなげません: %w", err)
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("VPN のサーバーとの通信が切れました: %w", err)
		}
		if loc, err := resp.Location(); err == nil && resp.StatusCode/100 == 3 {
			at = loc
			continue
		}
		var doc authDoc
		if resp.StatusCode != http.StatusOK || xml.Unmarshal(bytes.TrimSpace(raw), &doc) != nil {
			// ocserv は、そのユーザーが入れないグループを選ぶと、パスワードのあとに 401 を返す
			if resp.StatusCode == http.StatusUnauthorized && cfg.Group != "" {
				return "", fmt.Errorf("VPN のサーバーがログインを断りました (%s)。グループ %s を確かめてください", resp.Status, cfg.Group)
			}
			return "", fmt.Errorf("VPN のサーバーが AnyConnect のログインに答えません (%s)", resp.Status)
		}
		switch doc.Type {
		case "complete":
			if doc.SessionToken != "" {
				return doc.SessionToken, nil
			}
			for _, c := range jar.Cookies(at) {
				if c.Name == "webvpn" && c.Value != "" {
					return c.Value, nil
				}
			}
			return "", errors.New("VPN のサーバーがログインの cookie を返しません")
		case "auth-request":
		default:
			return "", fmt.Errorf("VPN のサーバーの返事を読めません (%s)", doc.Type)
		}
		reply, err := answer(cfg, doc.Auth, &passName)
		if err != nil {
			return "", err
		}
		body = query("auth-reply", doc.Opaque, reply)
		if a := doc.Auth.Form.Action; a != "" {
			if u, err := at.Parse(a); err == nil {
				at = u
			}
		}
	}
	return "", errors.New("VPN のサーバーのログインが終わりません")
}

// answer は、フォームの欄にユーザー名、パスワード、グループを入れた返事を作る
func answer(cfg Config, n authNode, passName *string) (string, error) {
	msg := strings.TrimSpace(n.Error + " " + n.Message)
	var auth, top strings.Builder
	filled := false
	for _, in := range n.Form.Inputs {
		var v string
		switch in.Type {
		case "text":
			v = cfg.User
		case "password":
			if *passName == in.Name {
				// 同じ欄をまた聞かれたら、さっきのパスワードが通らなかった
				return "", ErrAuth
			}
			if *passName != "" {
				return "", fmt.Errorf("VPN のサーバーがパスワードのほかの入力を求めています (%s)。DRAG ではまだ入れられません", msg)
			}
			v = cfg.Pass
			*passName = in.Name
		case "hidden":
			v = in.Value
		default:
			continue
		}
		elem(&auth, in.Name, v)
		filled = true
	}
	for _, s := range n.Form.Selects {
		if len(s.Options) == 0 {
			continue
		}
		v := s.Options[0].Value
		for _, o := range s.Options {
			if o.Selected != "" {
				v = o.Value
			}
		}
		if s.Name == "group_list" {
			if cfg.Group != "" {
				var names []string
				found := false
				for _, o := range s.Options {
					names = append(names, o.Value)
					if o.Value == cfg.Group || strings.TrimSpace(o.Label) == cfg.Group {
						v, found = o.Value, true
					}
				}
				if !found {
					return "", fmt.Errorf("VPN のサーバーにグループ %s がありません。選べるのは %s です", cfg.Group, strings.Join(names, ", "))
				}
			}
			elem(&top, "group-select", v)
		} else {
			elem(&auth, s.Name, v)
		}
		filled = true
	}
	if !filled {
		return "", fmt.Errorf("VPN のサーバーのログインに答えられません (%s)", msg)
	}
	return "<auth>" + auth.String() + "</auth>" + top.String(), nil
}
