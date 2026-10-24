package main

import (
	"errors"

	"github.com/asutarisucu/DRAG/app/internal/remote"
)

// サービスと Docker のウィンドウから呼ぶ窓口

func (a *App) Services(serverID string) ([]remote.Service, error) {
	s, err := a.server(serverID)
	if err != nil {
		return nil, err
	}
	return s.Services()
}

// ServiceAction はサービスを start・stop・restart する
func (a *App) ServiceAction(serverID, name, verb string) error {
	s, err := a.server(serverID)
	if err != nil {
		return err
	}
	return s.ServiceAction(name, verb)
}

func (a *App) Docker(serverID string) (remote.DockerInfo, error) {
	s, err := a.server(serverID)
	if err != nil {
		return remote.DockerInfo{}, err
	}
	return s.Docker()
}

// DockerAction は start・stop・restart・rm (コンテナ)、rmi・pull (イメージ)、volume-rm (ボリューム) を行う
func (a *App) DockerAction(serverID, verb, target string) error {
	s, err := a.server(serverID)
	if err != nil {
		return err
	}
	return s.DockerAction(verb, target)
}

// DockerVolumeSizes はボリュームごとの大きさ
func (a *App) DockerVolumeSizes(serverID string) (map[string]string, error) {
	s, err := a.server(serverID)
	if err != nil {
		return nil, err
	}
	return s.DockerVolumeSizes()
}

func (a *App) DockerMounts(serverID, name string) ([]remote.DockerMount, error) {
	s, err := a.server(serverID)
	if err != nil {
		return nil, err
	}
	return s.DockerMounts(name)
}

func (a *App) DockerStats(serverID string) ([]remote.DockerStat, error) {
	s, err := a.server(serverID)
	if err != nil {
		return nil, err
	}
	return s.DockerStats()
}

// LogStream は、サービス ("journal") かコンテナ ("docker") のログを "log:out" で送り始め、番号を返す。
// 止めるのは LogStop。止まったら "log:end" を送る
func (a *App) LogStream(serverID, kind, name string) (string, error) {
	s, err := a.server(serverID)
	if err != nil {
		return "", err
	}
	start := map[string]func(string, func([]byte), func(error)) (func(), error){
		"journal": s.JournalStream,
		"docker":  s.DockerLogStream,
	}[kind]
	if start == nil {
		return "", errors.New("ログの種類が違います")
	}
	return a.follow(func(onData func([]byte), onEnd func(error)) (func(), error) { return start(name, onData, onEnd) })
}
