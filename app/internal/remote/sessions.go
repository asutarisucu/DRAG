package remote

import (
	"strconv"
	"strings"
)

// Session は screen か tmux のセッション1つ
type Session struct {
	Kind     string `json:"kind"` // "screen" か "tmux"
	Name     string `json:"name"`
	Attached bool   `json:"attached"`
	Desc     string `json:"desc"`     // 一覧に出す説明。screen は作った日時、tmux はウィンドウの数
	LastUsed int64  `json:"lastUsed"` // 最後に使った時刻 (Unix 秒)。分からなければ 0
}

// 1つの exec で両方を調べる。screen は名前の前に PID が付くので、それごと名前にする (screen -r に渡せる)。
// screen の最後に使った時刻はソケットの更新日時で、つないだり切り離したりすると変わる。tmux は session_activity。
// screen -ls の最後の行は CRLF で終わるので、行末の空白を許す
const sessionsScript = `if command -v screen >/dev/null 2>&1; then
l=$(screen -ls 2>/dev/null)
echo "$l" | sed -n "s/^[[:space:]]*\([0-9][0-9]*\.[^[:space:]]*\)[[:space:]]*\(.*\)$/screen|\1|\2/p"
d=$(echo "$l" | sed -n "s/^[0-9][0-9]* Sockets\{0,1\} in \(\/[^[:space:]]*\)\.[[:space:]]*$/\1/p")
[ -n "$d" ] && for f in "$d"/*; do [ -e "$f" ] && echo "sock|${f##*/}|$(stat -c %Y "$f" 2>/dev/null)"; done
fi
if command -v tmux >/dev/null 2>&1; then tmux ls -F "tmux|#{session_name}|#{session_attached}|#{session_windows}|#{session_activity}" 2>/dev/null; fi
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
	used := map[string]int64{}
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
		case len(f) == 3 && f[0] == "sock":
			used[f[1]], _ = strconv.ParseInt(f[2], 10, 64)
		case len(f) == 5 && f[0] == "tmux":
			last, _ := strconv.ParseInt(f[4], 10, 64)
			list = append(list, Session{Kind: "tmux", Name: f[1], Attached: f[2] != "0", Desc: f[3], LastUsed: last})
		}
	}
	for i := range list {
		if list[i].Kind == "screen" {
			list[i].LastUsed = used[list[i].Name]
		}
	}
	return list, nil
}
