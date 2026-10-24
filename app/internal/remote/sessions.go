package remote

import (
	"strconv"
	"strings"
	"time"
)

// Session は screen か tmux のセッション1つ
type Session struct {
	Kind     string `json:"kind"` // "screen" か "tmux"
	Name     string `json:"name"`
	Attached bool   `json:"attached"`
	Desc     string `json:"desc"`     // 一覧に出す説明。screen は作った日時、tmux はウィンドウの数
	LastUsed int64  `json:"lastUsed"` // 最後に使った時刻 (Unix 秒)。分からなければ 0
	// User は、ほかのユーザーの screen のセッションのときの持ち主。ログインしたユーザーのものなら空
	User string `json:"user"`
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
	return append(list, s.otherScreens()...), nil
}

// othersScript は、ほかのユーザーの screen のソケットを並べる。1行目はログインしたユーザーの名前。
// ソケットはユーザーごとのフォルダ (/run/screen/S-<ユーザー>) にあり、フォルダは持ち主しか読めないので、
// root か sudo モードのときだけ見える。ソケットの持ち主の実行の権限が立っていれば、どこかで開いている
const othersScript = `IFS= read -r me
command -v screen >/dev/null 2>&1 || exit 0
base=$(screen -ls 2>/dev/null | sed -n "s/.* in \(\/[^[:space:]]*\)\/S-[^/[:space:]]*\.[[:space:]]*$/\1/p" | head -n 1)
[ -n "$base" ] || base=/run/screen
for d in "$base"/S-*; do
u=${d##*/S-}
[ "$u" = "$me" ] && continue
for f in "$d"/*; do [ -S "$f" ] && echo "other|$u|${f##*/}|$(stat -c "%a|%Y" "$f")"; done
done 2>/dev/null
exit 0`

// otherScreens は、見える範囲のほかのユーザーの screen のセッション。
// root でも sudo モードでもなければ見えないので読まない。sudo を数秒おきに呼ばないよう、20 秒は前の結果を使う
func (s *Server) otherScreens() []Session {
	if s.id.uid != 0 && !s.Sudo() {
		return nil
	}
	s.mu.Lock()
	c := s.others[s.dev]
	s.mu.Unlock()
	if c != nil && time.Since(c.at) < 20*time.Second {
		return c.list
	}
	cmd, stdin := s.command(othersScript, s.cfg.User+"\n")
	out, _ := s.combined(cmd, stdin)
	list := []Session{}
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(strings.TrimSpace(l), "|")
		if len(f) != 5 || f[0] != "other" {
			continue
		}
		mode, _ := strconv.ParseInt(f[3], 8, 32)
		last, _ := strconv.ParseInt(f[4], 10, 64)
		list = append(list, Session{Kind: "screen", Name: f[2], User: f[1], Attached: mode&0o100 != 0, LastUsed: last})
	}
	s.mu.Lock()
	s.others[s.dev] = &otherCache{at: time.Now(), list: list}
	s.mu.Unlock()
	return list
}

// otherCache は、端末ごとの otherScreens の前の結果
type otherCache struct {
	at   time.Time
	list []Session
}
