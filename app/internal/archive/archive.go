// Package archive は、PC に落とした圧縮ファイルの中身を、展開して書き出さずに読む。
// サーバーには何も書き出さない (企画書の「解凍して閲覧」)
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

var ErrUnsupported = errors.New("この形式の圧縮ファイルは開けません")

// Entry は圧縮ファイルの中の1件
type Entry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Dir  bool   `json:"dir"`
}

type format int

const (
	fmtZip format = iota
	fmtTar
	fmtTarGz
	fmtGz // tar ではない、1つのファイルを gzip にしたもの
)

func formatOf(name string) (format, error) {
	n := strings.ToLower(name)
	switch {
	case strings.HasSuffix(n, ".zip"), strings.HasSuffix(n, ".jar"):
		return fmtZip, nil
	case strings.HasSuffix(n, ".tar.gz"), strings.HasSuffix(n, ".tgz"):
		return fmtTarGz, nil
	case strings.HasSuffix(n, ".tar"):
		return fmtTar, nil
	case strings.HasSuffix(n, ".gz"):
		return fmtGz, nil
	}
	return 0, ErrUnsupported
}

// Supported は、開ける形式の名前か
func Supported(name string) bool {
	_, err := formatOf(name)
	return err == nil
}

// List は中身の一覧を返す。.gz の1つのファイルなら、gz を外した名前の1件だけ
func List(local, name string) ([]Entry, error) {
	f, err := formatOf(name)
	if err != nil {
		return nil, err
	}
	var out []Entry
	switch f {
	case fmtZip:
		zr, err := zip.OpenReader(local)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		for _, e := range zr.File {
			out = append(out, Entry{Path: strings.TrimSuffix(e.Name, "/"), Size: int64(e.UncompressedSize64), Dir: e.FileInfo().IsDir()})
		}
	case fmtTar, fmtTarGz:
		err := walkTar(local, f == fmtTarGz, func(h *tar.Header, _ io.Reader) (bool, error) {
			p := strings.TrimSuffix(strings.TrimPrefix(h.Name, "./"), "/")
			if p != "" && p != "." {
				out = append(out, Entry{Path: p, Size: h.Size, Dir: h.Typeflag == tar.TypeDir})
			}
			return false, nil
		})
		if err != nil {
			return nil, err
		}
	case fmtGz:
		out = append(out, Entry{Path: strings.TrimSuffix(path.Base(name), path.Ext(name)), Size: -1})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Read は中の1件を読む。limit を超えるものは読まない
func Read(local, name, entry string, limit int64) ([]byte, error) {
	f, err := formatOf(name)
	if err != nil {
		return nil, err
	}
	read := func(r io.Reader) ([]byte, error) {
		b, err := io.ReadAll(io.LimitReader(r, limit+1))
		if err != nil {
			return nil, err
		}
		if int64(len(b)) > limit {
			return nil, errors.New("ファイルが大きすぎます")
		}
		return b, nil
	}
	switch f {
	case fmtZip:
		zr, err := zip.OpenReader(local)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		for _, e := range zr.File {
			if e.Name == entry {
				rc, err := e.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return read(rc)
			}
		}
	case fmtTar, fmtTarGz:
		var out []byte
		err := walkTar(local, f == fmtTarGz, func(h *tar.Header, r io.Reader) (bool, error) {
			if strings.TrimPrefix(h.Name, "./") != entry {
				return false, nil
			}
			b, err := read(r)
			out = b
			return true, err
		})
		if err != nil || out != nil {
			return out, err
		}
	case fmtGz:
		fh, err := os.Open(local)
		if err != nil {
			return nil, err
		}
		defer fh.Close()
		gz, err := gzip.NewReader(fh)
		if err != nil {
			return nil, err
		}
		return read(gz)
	}
	return nil, os.ErrNotExist
}

// walkTar は tar の中を順に fn へ渡す。fn が true を返したら止める
func walkTar(local string, gz bool, fn func(*tar.Header, io.Reader) (bool, error)) error {
	fh, err := os.Open(local)
	if err != nil {
		return err
	}
	defer fh.Close()
	var r io.Reader = fh
	if gz {
		g, err := gzip.NewReader(fh)
		if err != nil {
			return err
		}
		defer g.Close()
		r = g
	}
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		stop, err := fn(h, tr)
		if err != nil || stop {
			return err
		}
	}
}
