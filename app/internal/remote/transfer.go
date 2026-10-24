package remote

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// Progress は送った量を知らせる。done と total はバイト数
type Progress func(done, total int64)

// counter は読んだ量を数え、Progress に渡す
type counter struct {
	r     io.Reader
	done  *int64
	total int64
	fn    Progress
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	*c.done += int64(n)
	c.fn(*c.done, c.total)
	return n, err
}

// LocalSize は、PC のファイルかフォルダの中身の合計の大きさ
func LocalSize(local string) (int64, error) {
	var total int64
	err := filepath.WalkDir(local, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			fi, err := d.Info()
			if err != nil {
				return err
			}
			total += fi.Size()
		}
		return nil
	})
	return total, err
}

// Upload は PC のファイルかフォルダを dir の中へ送り、置いたパスを返す。
// keep が真で同じ名前があれば番号を足した名前にする。偽なら上書きし、フォルダは中身を混ぜる
func (s *Server) Upload(local, dir string, keep bool, fn Progress) (string, error) {
	total, err := LocalSize(local)
	if err != nil {
		return "", err
	}
	name := filepath.Base(local)
	if keep {
		name = s.UniqueName(dir, name, false)
	}
	root := path.Join(dir, name)
	var done int64
	err = filepath.WalkDir(local, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(local, p)
		dst := root
		if rel != "." {
			dst = path.Join(root, filepath.ToSlash(rel))
		}
		switch {
		case d.IsDir():
			if fi, err := s.fs().Stat(dst); err == nil && fi.IsDir() {
				return nil
			}
			return s.fs().Mkdir(dst)
		case d.Type().IsRegular():
			src, err := os.Open(p)
			if err != nil {
				return err
			}
			defer src.Close()
			f, err := s.fs().OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
			if err != nil {
				return err
			}
			_, err = f.ReadFromWithConcurrency(&counter{r: src, done: &done, total: total, fn: fn}, 0)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			return err
		}
		// シンボリックリンクなどは送らない
		return nil
	})
	return root, err
}

// Download はサーバーのファイルを PC の local へ書く。フォルダなら zip にして書く
func (s *Server) Download(remote, local string, fn Progress) error {
	fi, err := s.fs().Stat(remote)
	if err != nil {
		return err
	}
	out, err := os.Create(local)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		var done int64
		f, err := s.fs().Open(remote)
		if err != nil {
			out.Close()
			return err
		}
		defer f.Close()
		_, err = io.Copy(out, &counter{r: f, done: &done, total: fi.Size(), fn: fn})
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		return err
	}
	err = s.zipDir(remote, out, fn)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// zipDir はフォルダの中身を zip にする。zip の中のパスはフォルダの名前から始める
func (s *Server) zipDir(remote string, w io.Writer, fn Progress) error {
	var total int64
	for walk := s.fs().Walk(remote); walk.Step(); {
		if walk.Err() == nil && walk.Stat().Mode().IsRegular() {
			total += walk.Stat().Size()
		}
	}
	zw := zip.NewWriter(w)
	base := path.Dir(remote)
	var done int64
	for walk := s.fs().Walk(remote); walk.Step(); {
		if err := walk.Err(); err != nil {
			return err
		}
		fi := walk.Stat()
		name := strings.TrimPrefix(walk.Path(), base+"/")
		if fi.IsDir() {
			if _, err := zw.Create(name + "/"); err != nil {
				return err
			}
			continue
		}
		if !fi.Mode().IsRegular() {
			continue
		}
		h, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		h.Name = name
		h.Method = zip.Deflate
		zf, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		f, err := s.fs().Open(walk.Path())
		if err != nil {
			return err
		}
		_, err = io.Copy(zf, &counter{r: f, done: &done, total: total, fn: fn})
		f.Close()
		if err != nil {
			return err
		}
	}
	return zw.Close()
}

// Props はプロパティの窓に出す中身
type Props struct {
	Type    string `json:"type"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	Owner   string `json:"owner"`
	Group   string `json:"group"`
	ModTime int64  `json:"modTime"`
	Target  string `json:"target"` // シンボリックリンクの先
}

// Stat は stat コマンドで持ち主の名前まで調べる。SFTP では持ち主は番号でしか分からない
func (s *Server) Stat(p string) (Props, error) {
	sess, err := s.ssh.NewSession()
	if err != nil {
		return Props{}, err
	}
	defer sess.Close()
	cmd, stdin := s.command(`IFS= read -r p; exec stat -c "%F|%s|%A|%U|%G|%Y" -- "$p"`, p+"\n")
	sess.Stdin = strings.NewReader(stdin)
	out, err := sess.CombinedOutput(cmd)
	if err != nil {
		return Props{}, errors.New(strings.TrimSpace(string(out)))
	}
	f := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(f) != 6 {
		return Props{}, fmt.Errorf("stat を読めない: %q", out)
	}
	size, _ := strconv.ParseInt(f[1], 10, 64)
	mt, _ := strconv.ParseInt(f[5], 10, 64)
	pr := Props{Type: f[0], Size: size, Mode: f[2], Owner: f[3], Group: f[4], ModTime: mt}
	if strings.HasPrefix(pr.Mode, "l") {
		pr.Target, _ = s.fs().ReadLink(p)
	}
	return pr, nil
}
