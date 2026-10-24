package remote

import (
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/pkg/sftp"
)

type fakeInfo struct {
	mode     fs.FileMode
	uid, gid uint32
}

func (f fakeInfo) Name() string       { return "f" }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return false }
func (f fakeInfo) Sys() any           { return &sftp.FileStat{UID: f.uid, GID: f.gid} }

// 書けるかは、root ならいつも書けるとし、ほかは持ち主・グループ・その他の書き込みの権限で決める
func TestWritable(t *testing.T) {
	user := identity{uid: 1001, gids: []int{1001, 27}}
	root := identity{uid: 0}
	unknown := identity{uid: -1}
	for _, c := range []struct {
		name string
		id   identity
		fi   os.FileInfo
		want bool
	}{
		{"root のファイルを root で", root, fakeInfo{0o600, 0, 0}, true},
		{"root のファイルを利用者で", user, fakeInfo{0o644, 0, 0}, false},
		{"自分のファイル", user, fakeInfo{0o644, 1001, 1001}, true},
		{"自分の読み取り専用のファイル", user, fakeInfo{0o444, 1001, 1001}, false},
		{"入っているグループが書ける", user, fakeInfo{0o664, 0, 27}, true},
		{"入っていないグループだけが書ける", user, fakeInfo{0o664, 0, 4}, false},
		{"誰でも書ける", user, fakeInfo{0o666, 0, 0}, true},
		{"ユーザーが分からない", unknown, fakeInfo{0o600, 0, 0}, true},
	} {
		if got := c.id.writable(c.fi); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}
