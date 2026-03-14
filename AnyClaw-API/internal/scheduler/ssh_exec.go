package scheduler

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/db"
	"golang.org/x/crypto/ssh"
)

const sshDialTimeout = 15 * time.Second

func sshConfig(host *db.Host) (*ssh.ClientConfig, error) {
	var auth []ssh.AuthMethod
	if host.SSHKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(host.SSHKey))
		if err != nil {
			return nil, fmt.Errorf("parse ssh key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if host.SSHPassword != "" {
		auth = append(auth, ssh.Password(host.SSHPassword))
	}
	if len(auth) == 0 {
		return nil, fmt.Errorf("ssh_key or ssh_password required")
	}
	return &ssh.ClientConfig{
		User:            host.SSHUser,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         sshDialTimeout,
	}, nil
}

func runSSH(host *db.Host, cmd string) (string, error) {
	config, err := sshConfig(host)
	if err != nil {
		return "", err
	}
	addr := net.JoinHostPort(host.Addr, strconv.Itoa(host.SSHPort))
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return "", fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()
	var out bytes.Buffer
	session.Stdout = &out
	session.Stderr = &out
	runErr := session.Run(cmd)
	outStr := strings.TrimSpace(out.String())
	if runErr != nil {
		if outStr != "" {
			return outStr, fmt.Errorf("%s: %s", runErr.Error(), outStr)
		}
		return "", runErr
	}
	return outStr, nil
}

// streamOutReader 包装 SSH 会话的 stdout，Close 时关闭连接
type streamOutReader struct {
	reader io.Reader
	client *ssh.Client
	sess   *ssh.Session
}

func (r *streamOutReader) Read(p []byte) (n int, err error) {
	return r.reader.Read(p)
}

func (r *streamOutReader) Close() error {
	if r.sess != nil {
		_ = r.sess.Close()
	}
	if r.client != nil {
		_ = r.client.Close()
	}
	return nil
}

// runSSHStreamOut 在宿主机上执行命令，返回 stdout 流（调用方需 Close）
func runSSHStreamOut(host *db.Host, cmd string) (io.ReadCloser, error) {
	config, err := sshConfig(host)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(host.Addr, strconv.Itoa(host.SSHPort))
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}
	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("new session: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, err
	}
	session.Stderr = nil
	if err := session.Start(cmd); err != nil {
		session.Close()
		client.Close()
		return nil, err
	}
	return &streamOutReader{reader: stdout, client: client, sess: session}, nil
}

// runSSHStreamIn 在宿主机上执行命令，将 stdin 作为输入
func runSSHStreamIn(host *db.Host, cmd string, stdin io.Reader) error {
	config, err := sshConfig(host)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(host.Addr, strconv.Itoa(host.SSHPort))
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	defer session.Close()
	session.Stdin = stdin
	session.Stdout = nil
	session.Stderr = nil
	if err := session.Start(cmd); err != nil {
		return err
	}
	return session.Wait()
}
