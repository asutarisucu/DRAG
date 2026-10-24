package remote

import (
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
)

// ログインしたユーザーの uid と、入っているグループ。ファイルに書けるかを、操作する前に画面に出すのに使う。
// 本当に書けるかはサーバーが決めるので、外れていても操作が失敗するだけで、そのときに sudo モードを求める
type identity struct {
	uid    int // 分からなければ -1
	gids   []int
	groups []string // グループの名前。journal や docker を sudo なしで使えるかを見る
}

func (s *Server) loadIdentity() {
	s.id = identity{uid: -1}
	out, err := s.outputPlain(`exec /bin/sh -c 'id -u; id -G; id -Gn'`)
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	uid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return
	}
	s.id.uid = uid
	if len(lines) > 1 {
		for _, g := range strings.Fields(lines[1]) {
			if n, err := strconv.Atoi(g); err == nil {
				s.id.gids = append(s.id.gids, n)
			}
		}
	}
	if len(lines) > 2 {
		s.id.groups = strings.Fields(lines[2])
	}
}

// writable は、ログインしたユーザーが fi に書けるか。root なら書ける。分からないときは書けるとして扱う
func (id identity) writable(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*sftp.FileStat)
	if id.uid < 0 || id.uid == 0 || !ok {
		return true
	}
	m := fi.Mode().Perm()
	switch {
	case int(st.UID) == id.uid:
		return m&0o200 != 0
	case slices.Contains(id.gids, int(st.GID)):
		return m&0o020 != 0
	default:
		return m&0o002 != 0
	}
}
