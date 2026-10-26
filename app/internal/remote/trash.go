package remote

import (
	"bufio"
	"cmp"
	"errors"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"
)

// TrashEntry はゴミ箱の中のもの1件。freedesktop.org のゴミ箱の info から読む
type TrashEntry struct {
	TrashItem
	Deleted int64 `json:"deleted"` // 削除した時刻 (Unix 秒)
	Dir     bool  `json:"dir"`
}

func (s *Server) trashRoot() (string, error) {
	home, err := s.Home()
	if err != nil {
		return "", err
	}
	return path.Join(home, ".local/share/Trash"), nil
}

// TrashList はゴミ箱の中を、新しく削除したものから返す
func (s *Server) TrashList() ([]TrashEntry, error) {
	root, err := s.trashRoot()
	if err != nil {
		return nil, err
	}
	infos, err := s.sftp.ReadDir(path.Join(root, "info"))
	if err != nil {
		return []TrashEntry{}, nil
	}
	list := []TrashEntry{}
	for _, fi := range infos {
		name, ok := strings.CutSuffix(fi.Name(), ".trashinfo")
		if !ok {
			continue
		}
		f, err := s.sftp.Open(path.Join(root, "info", fi.Name()))
		if err != nil {
			continue
		}
		e := TrashEntry{TrashItem: TrashItem{Name: name}}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			k, v, _ := strings.Cut(sc.Text(), "=")
			switch k {
			case "Path":
				if p, err := url.PathUnescape(v); err == nil {
					e.From = p
				}
			case "DeletionDate":
				if t, err := time.ParseInLocation("2006-01-02T15:04:05", v, time.Local); err == nil {
					e.Deleted = t.Unix()
				}
			}
		}
		f.Close()
		if st, err := s.fs().Lstat(path.Join(root, "files", name)); err == nil {
			e.Dir = st.IsDir()
		} else {
			// 中身の無い info は、ほかの道具が中身だけ消したもの
			continue
		}
		list = append(list, e)
	}
	// 削除した時刻は秒までなので、同じ秒のものは名前の順にする
	slices.SortFunc(list, func(a, b TrashEntry) int {
		return cmp.Or(cmp.Compare(b.Deleted, a.Deleted), cmp.Compare(a.Name, b.Name))
	})
	return list, nil
}

// TrashDelete はゴミ箱の中のものを完全に削除する。names が空ならゴミ箱を空にする
func (s *Server) TrashDelete(names []string) error {
	root, err := s.trashRoot()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		list, err := s.TrashList()
		if err != nil {
			return err
		}
		for _, e := range list {
			names = append(names, e.Name)
		}
	}
	var paths []string
	for _, n := range names {
		if n == "" || strings.Contains(n, "/") || n == "." || n == ".." {
			return errors.New("ゴミ箱の中の名前が違います")
		}
		paths = append(paths, path.Join(root, "files", n), path.Join(root, "info", n+".trashinfo"))
	}
	in, err := argLines(paths...)
	if err != nil {
		return err
	}
	// フォルダの中に読めないものがあっても消せるよう、rm を使う。パスは標準入力で渡す
	cmd, stdin := s.command(`while IFS= read -r p; do rm -rf -- "$p" || exit 1; done`, in)
	out, err := s.combined(cmd, stdin)
	if err != nil {
		if denied(out) {
			return ErrDenied
		}
		return errors.New(strings.TrimSpace(out))
	}
	return nil
}
