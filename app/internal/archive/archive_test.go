package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestZipTarGz(t *testing.T) {
	dir := t.TempDir()

	zp := filepath.Join(dir, "a.zip")
	zf, _ := os.Create(zp)
	zw := zip.NewWriter(zf)
	zw.Create("conf/")
	w, _ := zw.Create("conf/server.properties")
	w.Write([]byte("motd=こんにちは\n"))
	zw.Close()
	zf.Close()

	tp := filepath.Join(dir, "b.tar.gz")
	tf, _ := os.Create(tp)
	gw := gzip.NewWriter(tf)
	tw := tar.NewWriter(gw)
	tw.WriteHeader(&tar.Header{Name: "./mc/", Typeflag: tar.TypeDir, Mode: 0o755})
	body := []byte("[INFO]: Done\n")
	tw.WriteHeader(&tar.Header{Name: "./mc/latest.log", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))})
	tw.Write(body)
	tw.Close()
	gw.Close()
	tf.Close()

	gp := filepath.Join(dir, "c.sql.gz")
	gf, _ := os.Create(gp)
	g := gzip.NewWriter(gf)
	g.Write([]byte("CREATE TABLE x;\n"))
	g.Close()
	gf.Close()

	for _, c := range []struct{ local, name, entry, want string }{
		{zp, "a.zip", "conf/server.properties", "motd=こんにちは\n"},
		{tp, "b.tar.gz", "mc/latest.log", "[INFO]: Done\n"},
		{gp, "c.sql.gz", "c.sql", "CREATE TABLE x;\n"},
	} {
		list, err := List(c.local, c.name)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		found := false
		for _, e := range list {
			if e.Path == c.entry && !e.Dir {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s の一覧 %+v", c.name, list)
		}
		b, err := Read(c.local, c.name, c.entry, 1<<20)
		if err != nil || string(b) != c.want {
			t.Fatalf("%s: %q %v", c.name, b, err)
		}
	}
	if Supported("x.7z") {
		t.Fatal("7z は開けない")
	}
}
