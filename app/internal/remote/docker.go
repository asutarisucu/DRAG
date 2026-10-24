package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Docker のコンテナ・イメージ・ボリューム。docker の CLI を --format '{{json .}}' で呼んで読む。
// キーと値の形は docker/cli@v27.5.1 cli/command/formatter/container.go・image.go・volume.go と
// cli/command/container/formatter_stats.go のメソッドに合わせる (値はどれも文字列)

type Container struct {
	ID      string `json:"ID"`
	Names   string `json:"Names"`
	Image   string `json:"Image"`
	State   string `json:"State"`  // running・exited など
	Status  string `json:"Status"` // Up 3 days・Exited (0) 2 hours ago など
	Ports   string `json:"Ports"`
	Labels  string `json:"Labels"`
	Mounts  string `json:"Mounts"`
	Project string `json:"project"` // Compose のプロジェクト。Labels から取る
	Compose string `json:"compose"` // Compose のファイル
}

type Image struct {
	ID         string `json:"ID"`
	Repository string `json:"Repository"`
	Tag        string `json:"Tag"`
	Size       string `json:"Size"`
	Created    string `json:"CreatedSince"`
}

type Volume struct {
	Name       string `json:"Name"`
	Driver     string `json:"Driver"`
	Mountpoint string `json:"Mountpoint"`
}

type DockerInfo struct {
	Containers []Container `json:"containers"`
	Images     []Image     `json:"images"`
	Volumes    []Volume    `json:"volumes"`
}

// ErrNoDocker は docker の CLI が無いとき
var ErrNoDocker = errors.New("このサーバーには Docker が入っていません")

const dockerScript = `command -v docker >/dev/null 2>&1 || { echo "__NO_DOCKER__"; exit 0; }
echo "__C__"; docker ps -a --no-trunc --format "{{json .}}" || exit 1
echo "__I__"; docker images --format "{{json .}}" || exit 1
echo "__V__"; docker volume ls --format "{{json .}}" || exit 1`

// Docker はコンテナ・イメージ・ボリュームの一覧。docker を使う権限が無ければ ErrDenied
func (s *Server) Docker() (DockerInfo, error) {
	cmd, stdin := s.command(dockerScript, "")
	out, err := s.combined(cmd, stdin)
	if strings.Contains(out, "__NO_DOCKER__") {
		return DockerInfo{}, ErrNoDocker
	}
	if err != nil {
		if denied(out) {
			return DockerInfo{}, ErrDenied
		}
		return DockerInfo{}, errors.New(strings.TrimSpace(out))
	}
	info := DockerInfo{Containers: []Container{}, Images: []Image{}, Volumes: []Volume{}}
	part := ""
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		switch l {
		case "__C__", "__I__", "__V__":
			part = l
			continue
		case "":
			continue
		}
		var err error
		switch part {
		case "__C__":
			var c Container
			if err = json.Unmarshal([]byte(l), &c); err == nil {
				c.Project, c.Compose = label(c.Labels, "com.docker.compose.project"), label(c.Labels, "com.docker.compose.project.config_files")
				info.Containers = append(info.Containers, c)
			}
		case "__I__":
			var i Image
			if err = json.Unmarshal([]byte(l), &i); err == nil {
				info.Images = append(info.Images, i)
			}
		case "__V__":
			var v Volume
			if err = json.Unmarshal([]byte(l), &v); err == nil {
				info.Volumes = append(info.Volumes, v)
			}
		}
	}
	return info, nil
}

// label は "a=1,b=2" の形の Labels から name の値を取る。値に , を含むものは考えない
func label(labels, name string) string {
	for _, kv := range strings.Split(labels, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v
		}
	}
	return ""
}

// DockerAction はコンテナ・イメージ・ボリュームを操作する。target は操作する名前か ID
func (s *Server) DockerAction(verb, target string) error {
	args, ok := map[string]string{
		"start": "start", "stop": "stop", "restart": "restart", "rm": "rm",
		"rmi": "rmi", "volume-rm": "volume rm", "pull": "pull",
	}[verb]
	if !ok {
		return fmt.Errorf("できない操作です: %s", verb)
	}
	cmd, stdin := s.command(`IFS= read -r t; exec docker `+args+` -- "$t"`, target+"\n")
	out, err := s.combined(cmd, stdin)
	if err == nil {
		return nil
	}
	if denied(out) {
		return ErrDenied
	}
	return errors.New(strings.TrimSpace(out))
}

// DockerVolumeSizes はボリュームごとの大きさ。docker system df -v で数えるので時間がかかる。
// 値は docker/cli@v27.5.1 cli/command/formatter/volume.go の Size (units.HumanSize の書き方)。分からなければ N/A
func (s *Server) DockerVolumeSizes() (map[string]string, error) {
	cmd, stdin := s.command(`exec docker system df -v --format "{{range .Volumes}}{{.Name}} {{.Size}}{{println}}{{end}}"`, "")
	out, err := s.combined(cmd, stdin)
	if err != nil {
		if denied(out) {
			return nil, ErrDenied
		}
		return nil, errors.New(strings.TrimSpace(out))
	}
	sizes := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if name, size, ok := strings.Cut(strings.TrimSpace(l), " "); ok {
			sizes[name] = size
		}
	}
	return sizes, nil
}

// DockerMount はコンテナのマウント1つ。moby@v27.5.1 api/types/types.go の MountPoint と同じキー
type DockerMount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
}

// DockerMounts はコンテナのマウント
func (s *Server) DockerMounts(name string) ([]DockerMount, error) {
	cmd, stdin := s.command(`IFS= read -r c; exec docker inspect --format "{{json .Mounts}}" -- "$c"`, name+"\n")
	out, err := s.combined(cmd, stdin)
	if err != nil {
		if denied(out) {
			return nil, ErrDenied
		}
		return nil, errors.New(strings.TrimSpace(out))
	}
	list := []DockerMount{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &list); err != nil {
		return nil, err
	}
	return list, nil
}

// DockerStat は動いているコンテナ1つの CPU とメモリ
type DockerStat struct {
	Name     string `json:"Name"`
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
}

// DockerStats は動いているコンテナの CPU とメモリ。docker stats は1回の計測に1秒ほどかかる
func (s *Server) DockerStats() ([]DockerStat, error) {
	cmd, stdin := s.command(`exec docker stats --no-stream --format "{{json .}}"`, "")
	out, err := s.combined(cmd, stdin)
	if err != nil {
		if denied(out) {
			return nil, ErrDenied
		}
		return nil, errors.New(strings.TrimSpace(out))
	}
	list := []DockerStat{}
	for _, l := range strings.Split(out, "\n") {
		var st DockerStat
		if json.Unmarshal([]byte(strings.TrimSpace(l)), &st) == nil && st.Name != "" {
			list = append(list, st)
		}
	}
	return list, nil
}

// DockerLogStream はコンテナのログの直近の行から追記までを onData に渡す。
// コンテナの標準エラーもログなので一緒に流す。そのため権限の失敗は先に調べ、sudo なしで使えなければ ErrDenied を返す
func (s *Server) DockerLogStream(name string, onData func([]byte), onEnd func(error)) (func(), error) {
	if !s.canUseDocker() && !s.Sudo() {
		return nil, ErrDenied
	}
	return s.stream(`IFS= read -r c; exec docker logs --tail 300 -f -- "$c" 2>&1`, name+"\n", onData, onEnd)
}

// canUseDocker は、sudo なしで docker の CLI を使えるか。root か docker のグループ
func (s *Server) canUseDocker() bool {
	return s.id.uid == 0 || slices.Contains(s.id.groups, "docker")
}
