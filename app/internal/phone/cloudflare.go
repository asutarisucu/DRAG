package phone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DNSProvider は、DNS-01 の検証に使う TXT レコードを置き、消す関数を返す。name は _acme-challenge.<ドメイン> の形
type DNSProvider interface {
	SetTXT(ctx context.Context, name, value string) (remove func(), err error)
}

// Cloudflare は Cloudflare の DNS に TXT レコードを置く。
// API は developers.cloudflare.com/api の zones の list と、dns/records の create・delete。
// トークンには、そのゾーンの Zone Read (ゾーンを探す) と DNS Edit (レコードを書く) が要る
type Cloudflare struct {
	Token  string
	API    string       // 空なら https://api.cloudflare.com/client/v4。試すときに差し替える
	Client *http.Client // 空なら http.DefaultClient
}

// call は API を呼び、result を out に読む。success が false なら errors の文をまとめて返す
func (c *Cloudflare) call(ctx context.Context, method, path string, body, out any) error {
	api := c.API
	if api == "" {
		api = "https://api.cloudflare.com/client/v4"
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, api+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.Client
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("Cloudflare に届きません: %w", err)
	}
	defer res.Body.Close()
	var r struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil || !r.Success {
		msgs := []string{}
		for _, e := range r.Errors {
			msgs = append(msgs, fmt.Sprintf("%s (%d)", e.Message, e.Code))
		}
		if len(msgs) == 0 {
			msgs = append(msgs, res.Status)
		}
		return fmt.Errorf("Cloudflare: %s", strings.Join(msgs, "; "))
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

// zone は name が入るゾーンの ID を探す。drag.example.com なら drag.example.com、example.com の順に聞く
func (c *Cloudflare) zone(ctx context.Context, name string) (string, error) {
	labels := strings.Split(strings.TrimSuffix(name, "."), ".")
	for i := 0; i < len(labels)-1; i++ {
		var zones []struct {
			ID string `json:"id"`
		}
		if err := c.call(ctx, "GET", "/zones?name="+url.QueryEscape(strings.Join(labels[i:], ".")), nil, &zones); err != nil {
			return "", err
		}
		if len(zones) > 0 {
			return zones[0].ID, nil
		}
	}
	return "", fmt.Errorf("Cloudflare に %s のゾーンが見つかりません。トークンにそのゾーンの Zone Read を付けてください", name)
}

func (c *Cloudflare) SetTXT(ctx context.Context, name, value string) (func(), error) {
	zone, err := c.zone(ctx, strings.TrimPrefix(name, "_acme-challenge."))
	if err != nil {
		return nil, err
	}
	var rec struct {
		ID string `json:"id"`
	}
	body := map[string]any{"type": "TXT", "name": name, "content": value, "ttl": 60}
	if err := c.call(ctx, "POST", "/zones/"+zone+"/dns_records", body, &rec); err != nil {
		return nil, err
	}
	return func() {
		// 取り終えたあとの片付けなので、取るときの ctx が切れていても消す
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c.call(ctx, "DELETE", "/zones/"+zone+"/dns_records/"+rec.ID, nil, nil)
	}, nil
}
