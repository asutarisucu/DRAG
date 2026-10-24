package remote

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// ErrExists は、作る先や移す先に同じ名前のものがあるときに返す
var ErrExists = errors.New("同じ名前のものがもうあります")

func (s *Server) exists(p string) bool {
	_, err := s.fs().Lstat(p)
	return err == nil
}

// UniqueName は dir の中で使われていない名前を返す。
// copy が真なら「名前 - コピー.txt」、偽なら「名前 (2).txt」の形で番号を足す (mock/ と同じ)
func (s *Server) UniqueName(dir, name string, copy bool) string {
	if !s.exists(path.Join(dir, name)) {
		return name
	}
	base, ext := name, ""
	if i := strings.LastIndex(name, "."); i > 0 {
		base, ext = name[:i], name[i:]
	}
	for i := 1; ; i++ {
		var n string
		switch {
		case copy && i == 1:
			n = base + " - コピー" + ext
		case copy:
			n = fmt.Sprintf("%s - コピー (%d)%s", base, i, ext)
		default:
			n = fmt.Sprintf("%s (%d)%s", base, i+1, ext)
		}
		if !s.exists(path.Join(dir, n)) {
			return n
		}
	}
}

// CreateFile は空のファイルを作る。同じ名前があれば ErrExists
func (s *Server) CreateFile(p string) error {
	f, err := s.fs().OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		if s.exists(p) {
			return ErrExists
		}
		return err
	}
	return f.Close()
}

func (s *Server) Mkdir(p string) error {
	if s.exists(p) {
		return ErrExists
	}
	return s.fs().Mkdir(p)
}

// Rename は名前を変えるか、別のフォルダへ移す。移す先に同じ名前があれば ErrExists
func (s *Server) Rename(from, to string) error {
	if s.exists(to) {
		return ErrExists
	}
	return s.fs().Rename(from, to)
}

// Copy はファイルやフォルダを複製する。SFTP には複製の命令が無いので、サーバーの cp を使う。
// パスは引用符の扱いがシェルごとに違うので、コマンドに埋めずに標準入力で渡す
func (s *Server) Copy(from, to string) error {
	if s.exists(to) {
		return ErrExists
	}
	sess, err := s.ssh.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	cmd, stdin := s.command(`IFS= read -r s; IFS= read -r d; exec cp -a -- "$s" "$d"`, from+"\n"+to+"\n")
	sess.Stdin = strings.NewReader(stdin)
	out, err := sess.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// TrashItem はゴミ箱に移したもの1件
type TrashItem struct {
	Name string `json:"name"` // ゴミ箱の中での名前
	From string `json:"from"` // 元のパス
}

// Trash はゴミ箱へ移す。freedesktop.org のゴミ箱の決まりに合わせて ~/.local/share/Trash に置くので、
// サーバーのデスクトップ環境や gio trash と同じゴミ箱になる。
// sudo モードでも、ゴミ箱のフォルダと info のファイルは root の持ち物にしないよう、ふだんの SFTP で作る
func (s *Server) Trash(p string) (TrashItem, error) {
	home, err := s.Home()
	if err != nil {
		return TrashItem{}, err
	}
	root := path.Join(home, ".local/share/Trash")
	if err := s.sftp.MkdirAll(path.Join(root, "files")); err != nil {
		return TrashItem{}, err
	}
	if err := s.sftp.MkdirAll(path.Join(root, "info")); err != nil {
		return TrashItem{}, err
	}
	name := path.Base(p)
	// info のファイルを O_EXCL で作れた名前を、ゴミ箱の中での名前にする
	var info string
	for i := 1; ; i++ {
		n := name
		if i > 1 {
			n = fmt.Sprintf("%s.%d", name, i)
		}
		info = path.Join(root, "info", n+".trashinfo")
		f, err := s.sftp.OpenFile(info, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		if err != nil {
			if s.exists(info) {
				continue
			}
			return TrashItem{}, err
		}
		u := (&url.URL{Path: p}).EscapedPath()
		_, err = fmt.Fprintf(f, "[Trash Info]\nPath=%s\nDeletionDate=%s\n", u, time.Now().Format("2006-01-02T15:04:05"))
		f.Close()
		if err != nil {
			s.sftp.Remove(info)
			return TrashItem{}, err
		}
		name = n
		break
	}
	if err := s.fs().Rename(p, path.Join(root, "files", name)); err != nil {
		s.sftp.Remove(info)
		return TrashItem{}, fmt.Errorf("ゴミ箱へ移せません: %w", err)
	}
	return TrashItem{Name: name, From: p}, nil
}

// Restore はゴミ箱から元の場所へ戻す。元の場所に同じ名前があれば番号を足した名前にする
func (s *Server) Restore(item TrashItem) (string, error) {
	home, err := s.Home()
	if err != nil {
		return "", err
	}
	root := path.Join(home, ".local/share/Trash")
	dir := path.Dir(item.From)
	to := path.Join(dir, s.UniqueName(dir, path.Base(item.From), false))
	if err := s.fs().Rename(path.Join(root, "files", item.Name), to); err != nil {
		return "", err
	}
	if err := s.sftp.Remove(path.Join(root, "info", item.Name+".trashinfo")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return to, err
	}
	return to, nil
}
