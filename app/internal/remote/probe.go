package remote

import (
	"bufio"
	"strings"
)

// Info は、接続したときに調べたサーバーの様子。ウィンドウで使える機能を決めるのに使う
type Info struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Distro  string `json:"distro"`
	Systemd bool   `json:"systemd"`
	Docker  bool   `json:"docker"`
	Screen  bool   `json:"screen"`
	Tmux    bool   `json:"tmux"`
}

// ログインシェルが fish などでも同じに動くよう、sh を挟んで1行ずつ key=value で出す
const probeScript = `echo "os=$(uname -s)"; echo "arch=$(uname -m)"
if [ -r /etc/os-release ]; then . /etc/os-release; echo "distro=$NAME $VERSION_ID"; fi
[ -d /run/systemd/system ] && echo systemd=1
command -v docker >/dev/null 2>&1 && echo docker=1
command -v screen >/dev/null 2>&1 && echo screen=1
command -v tmux >/dev/null 2>&1 && echo tmux=1
exit 0`

// Probe はサーバーの OS と、systemd・docker・screen・tmux の有無を調べる
func (s *Server) Probe() (Info, error) {
	sess, err := s.ssh.NewSession()
	if err != nil {
		return Info{}, err
	}
	defer sess.Close()
	out, err := sess.Output("exec /bin/sh -c '" + probeScript + "'")
	if err != nil {
		return Info{}, err
	}
	var info Info
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		k, v, _ := strings.Cut(sc.Text(), "=")
		switch k {
		case "os":
			info.OS = v
		case "arch":
			info.Arch = v
		case "distro":
			info.Distro = strings.TrimSpace(v)
		case "systemd":
			info.Systemd = true
		case "docker":
			info.Docker = true
		case "screen":
			info.Screen = true
		case "tmux":
			info.Tmux = true
		}
	}
	return info, nil
}
