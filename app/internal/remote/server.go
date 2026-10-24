// Package remote は、PC アプリがサーバーと SSH と SFTP でやり取りする部分。
// サーバーには何も置かない
package remote

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Config はサーバーへの接続の設定
type Config struct {
	Host     string
	Port     int
	User     string
	Password string // パスワード。鍵ファイルを使うときは鍵のパスフレーズ
	KeyFile  string // 空ならパスワードで入る
}

// HostKeyError は、サーバーの鍵が known_hosts に無いか、登録したものと違うときに返す
type HostKeyError struct {
	Fingerprint string // SHA256:...
	KeyType     string
	Mismatch    bool // 登録した鍵と違う。なりすましの恐れがある
}

func (e *HostKeyError) Error() string {
	if e.Mismatch {
		return "サーバーの鍵が、前に登録したものと違います (" + e.Fingerprint + ")"
	}
	return "サーバーの鍵がまだ登録されていません (" + e.Fingerprint + ")"
}

// Server は1台のサーバーとの接続を、1つの端末から見たもの。
// 接続は PC とスマホで共有し、sudo モードだけを端末ごとに持つ
type Server struct {
	*link
	dev string // どの端末の sudo モードを使うか。PC の画面なら空
}

// link は1台のサーバーとの接続。SSH の上に SFTP を1本持つ
type link struct {
	cfg  Config
	ssh  *ssh.Client
	sftp *sftp.Client
	mon  *monitor // 見張れないサーバーでは nil

	mu     sync.Mutex
	closed bool
	sudo   map[string]*sudoMode // 端末ごとの sudo モード。OFF の端末は入らない
}

// As は、同じ接続を dev の端末から見たものを返す
func (s *Server) As(dev string) *Server {
	return &Server{link: s.link, dev: dev}
}

// Closed は、閉じたか切れたあとに閉じた接続か
func (s *Server) Closed() bool { return s.isClosed() }

func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// knownHostsMu は known_hosts への書き足しを1つずつにする
var knownHostsMu sync.Mutex

// hostKeyCallback は known_hosts で鍵を確かめる。
// 登録されていない鍵は、trust に同じ指紋が渡されたときだけ受け入れて書き足す
func hostKeyCallback(path, trust string) (ssh.HostKeyCallback, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// knownhosts.New はファイルが無いと失敗するので、先に空のファイルを作る
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return nil, err
	}
	f.Close()
	check, err := knownhosts.New(path)
	if err != nil {
		return nil, err
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := check(hostname, remote, key)
		var ke *knownhosts.KeyError
		if err == nil || !errors.As(err, &ke) {
			return err
		}
		fp := ssh.FingerprintSHA256(key)
		if len(ke.Want) > 0 {
			return &HostKeyError{Fingerprint: fp, KeyType: key.Type(), Mismatch: true}
		}
		if trust != fp {
			return &HostKeyError{Fingerprint: fp, KeyType: key.Type()}
		}
		knownHostsMu.Lock()
		defer knownHostsMu.Unlock()
		w, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer w.Close()
		_, err = fmt.Fprintln(w, knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key))
		return err
	}, nil
}

func authMethods(cfg Config) ([]ssh.AuthMethod, error) {
	if cfg.KeyFile == "" {
		pw := cfg.Password
		return []ssh.AuthMethod{
			ssh.Password(pw),
			// パスワードを keyboard-interactive でしか受け付けないサーバーがある
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				ans := make([]string, len(questions))
				for i := range ans {
					ans[i] = pw
				}
				return ans, nil
			}),
		}, nil
	}
	b, err := os.ReadFile(cfg.KeyFile)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(b)
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(b, []byte(cfg.Password))
	}
	if err != nil {
		return nil, err
	}
	return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
}

// Dial はサーバーにつなぎ、SFTP を開く。
// サーバーの鍵が未登録なら *HostKeyError を返す。利用者が確かめた指紋を trust に渡して呼び直すと登録する
func Dial(cfg Config, knownHostsPath, trust string) (*Server, error) {
	auth, err := authMethods(cfg)
	if err != nil {
		return nil, err
	}
	hk, err := hostKeyCallback(knownHostsPath, trust)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	sc, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: hk,
		Timeout:         10 * time.Second,
	})
	if err != nil {
		var he *HostKeyError
		if errors.As(err, &he) {
			return nil, he
		}
		return nil, err
	}
	fc, err := sftp.NewClient(sc)
	if err != nil {
		sc.Close()
		return nil, fmt.Errorf("SFTP を開けません: %w", err)
	}
	s := &Server{link: &link{cfg: cfg, ssh: sc, sftp: fc, sudo: map[string]*sudoMode{}}}
	// 見張れなくても、ターミナルとファイルは使える
	s.startMonitor()
	return s, nil
}

// Wait は接続が切れるまで待つ。
// 回線が黙って切れたときも気づけるように、interval ごとに keepalive を送り、timeout までに返事がなければ閉じる
func (s *Server) Wait(interval, timeout time.Duration) error {
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
			}
			done := make(chan error, 1)
			go func() {
				_, _, err := s.ssh.SendRequest("keepalive@openssh.com", true, nil)
				done <- err
			}()
			select {
			case err := <-done:
				if err == nil {
					continue
				}
			case <-time.After(timeout):
			case <-stop:
				return
			}
			s.ssh.Close()
			return
		}
	}()
	return s.ssh.Wait()
}

func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	for _, m := range s.sudo {
		m.close()
	}
	clear(s.sudo)
	s.sftp.Close()
	s.ssh.Close()
}

// Home はログインしたユーザーのホームのパス。sftp-server はホームで起動する
func (s *Server) Home() (string, error) {
	return s.sftp.Getwd()
}
