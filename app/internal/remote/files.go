package remote

import (
	"os"
	"path"
	"slices"
	"strings"
)

// Entry はフォルダの中の1件
type Entry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Dir     bool   `json:"dir"`
	Link    bool   `json:"link"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modTime"` // Unix 秒
	Locked  bool   `json:"locked"`  // ログインしたユーザーでは書けない。変えるには sudo モードが要る
}

// ListDir はフォルダの中を、フォルダを先にして名前の順で返す。
// シンボリックリンクは、先がフォルダならフォルダとして扱う
func (s *Server) ListDir(dir string) ([]Entry, error) {
	infos, err := s.fs().ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(infos))
	for _, fi := range infos {
		e := Entry{
			Name:    fi.Name(),
			Path:    path.Join(dir, fi.Name()),
			Dir:     fi.IsDir(),
			Size:    fi.Size(),
			ModTime: fi.ModTime().Unix(),
		}
		e.Locked = !s.id.writable(fi)
		if fi.Mode()&os.ModeSymlink != 0 {
			e.Link = true
			if t, err := s.fs().Stat(e.Path); err == nil {
				e.Dir = t.IsDir()
			}
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b Entry) int {
		if a.Dir != b.Dir {
			if a.Dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out, nil
}
