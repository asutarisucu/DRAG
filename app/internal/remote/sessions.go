package remote

import (
	"strings"
)

// Session は screen か tmux のセッション1つ
type Session struct {
	Kind     string `json:"kind"` // "screen" か "tmux"
	Name     string `json:"name"`
	Attached bool   `json:"attached"`
	Desc     string `json:"desc"` // 一覧に出す説明。screen は作った日時、tmux はウィンドウの数
}

// 1つの exec で両方を調べる。screen は名前の前に PID が付くので、それごと名前にする (screen -r に渡せる)
const sessionsScript = `if command -v screen >/dev/null 2>&1; then screen -ls 2>/dev/null | sed -n "s/^[[:space:]]*\([0-9][0-9]*\.[^[:space:]]*\)[[:space:]]*\(.*\)$/screen|\1|\2/p"; fi
if command -v tmux >/dev/null 2>&1; then tmux ls -F "tmux|#{session_name}|#{session_attached}|#{session_windows}" 2>/dev/null; fi
exit 0`

// Sessions は screen と tmux のセッションの一覧を返す。入っていなければ空
func (s *Server) Sessions() ([]Session, error) {
	sess, err := s.ssh.NewSession()
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	out, err := sess.Output("exec /bin/sh -c '" + sessionsScript + "'")
	if err != nil {
		return nil, err
	}
	list := []Session{}
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(l, "|")
		switch {
		case len(f) == 3 && f[0] == "screen":
			// 例: (10/24/26 12:43:26)	(Detached)
			rest := strings.Fields(strings.NewReplacer("(", " ", ")", " ").Replace(f[2]))
			attached := strings.Contains(f[2], "(Attached)")
			desc := strings.Join(rest, " ")
			desc = strings.TrimSpace(strings.NewReplacer("Attached", "", "Detached", "").Replace(desc))
			list = append(list, Session{Kind: "screen", Name: f[1], Attached: attached, Desc: desc})
		case len(f) == 4 && f[0] == "tmux":
			list = append(list, Session{Kind: "tmux", Name: f[1], Attached: f[2] != "0", Desc: f[3]})
		}
	}
	return list, nil
}
