package remote

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrDenied は、ログインしたユーザーの権限では扱えないとき。sudo モードにすれば扱える。
// 画面は文の "permission denied" を見て sudo モードを求める
var ErrDenied = errors.New("permission denied")

// Service は systemd のサービス1つ
type Service struct {
	Name   string `json:"name"` // nginx.service
	Desc   string `json:"desc"`
	Active string `json:"active"` // active・inactive・failed・activating など
	Sub    string `json:"sub"`    // running・exited・dead など
}

// list-units は一度も動かしていないサービスを読み込んでいないので出さない。
// 入れてあるサービスは list-unit-files から足す。そちらには説明が無い
const servicesScript = `command -v systemctl >/dev/null 2>&1 || exit 3
systemctl list-units --type=service --all --plain --no-legend --no-pager 2>/dev/null
echo __FILES__
systemctl list-unit-files --type=service --state=enabled,disabled --no-legend --no-pager 2>/dev/null`

// Services はサービスの一覧。systemd が無ければ空
func (s *Server) Services() ([]Service, error) {
	out, err := s.outputPlain("exec /bin/sh -c '" + servicesScript + "'")
	if err != nil && out == "" {
		return []Service{}, nil
	}
	units, files, _ := strings.Cut(out, "__FILES__\n")
	list := []Service{}
	seen := map[string]bool{}
	for _, l := range strings.Split(units, "\n") {
		// UNIT LOAD ACTIVE SUB DESCRIPTION の順
		f := strings.Fields(l)
		if len(f) < 4 || f[1] != "loaded" || !strings.HasSuffix(f[0], ".service") {
			continue
		}
		seen[f[0]] = true
		list = append(list, Service{Name: f[0], Active: f[2], Sub: f[3], Desc: strings.Join(f[4:], " ")})
	}
	for _, l := range strings.Split(files, "\n") {
		// UNIT FILE STATE の順。テンプレート (名前@.service) は動かせないので外す
		f := strings.Fields(l)
		if len(f) < 2 || seen[f[0]] || !strings.HasSuffix(f[0], ".service") || strings.Contains(f[0], "@.") {
			continue
		}
		list = append(list, Service{Name: f[0], Active: "inactive", Sub: "dead"})
	}
	return list, nil
}

// ServiceAction はサービスを起動・停止・再起動する。sudo モードなら sudo を付ける
func (s *Server) ServiceAction(name, verb string) error {
	if !slices.Contains([]string{"start", "stop", "restart"}, verb) {
		return fmt.Errorf("できない操作です: %s", verb)
	}
	in, err := argLines(name)
	if err != nil {
		return err
	}
	cmd, stdin := s.command(`IFS= read -r u; exec systemctl `+verb+` -- "$u"`, in)
	out, err := s.combined(cmd, stdin)
	if err == nil {
		return nil
	}
	if denied(out) {
		return ErrDenied
	}
	return errors.New(strings.TrimSpace(out))
}

// denied は、コマンドの出力が権限の足りなさを言っているか
func denied(out string) bool {
	for _, k := range []string{"Interactive authentication required", "Access denied", "access denied", "Permission denied", "permission denied"} {
		if strings.Contains(out, k) {
			return true
		}
	}
	return false
}

func (s *Server) combined(cmd, stdin string) (string, error) {
	sess, err := s.ssh.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	sess.Stdin = strings.NewReader(stdin)
	out, err := sess.CombinedOutput(cmd)
	return string(out), err
}

// canReadJournal は、sudo なしでほかのユーザーとシステムのログを journalctl で読めるか
func (s *Server) canReadJournal() bool {
	return s.id.uid == 0 || slices.ContainsFunc(s.id.groups, func(g string) bool { return g == "adm" || g == "systemd-journal" || g == "wheel" })
}

// JournalStream は、サービスのログの直近の行から追記までを onData に渡す。
// ログインしたユーザーでシステムのログを読めず、sudo モードでもなければ ErrDenied を返す
func (s *Server) JournalStream(unit string, onData func([]byte), onEnd func(error)) (func(), error) {
	if !s.canReadJournal() && !s.Sudo() {
		return nil, ErrDenied
	}
	in, err := argLines(unit)
	if err != nil {
		return nil, err
	}
	return s.stream(`IFS= read -r u; exec journalctl -u "$u" -n 300 -f --no-pager -o short 2>&1`, in, onData, onEnd)
}
