package scheduler

import (
	"bytes"
	"fmt"
	"net"
	"strconv"

	"github.com/anyclaw/anyclaw-api/internal/db"
	"golang.org/x/crypto/ssh"
)

func runSSH(host *db.Host, cmd string) (string, error) {
	var auth []ssh.AuthMethod
	if host.SSHKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(host.SSHKey))
		if err != nil {
			return "", fmt.Errorf("parse ssh key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if host.SSHPassword != "" {
		auth = append(auth, ssh.Password(host.SSHPassword))
	}
	if len(auth) == 0 {
		return "", fmt.Errorf("ssh_key or ssh_password required")
	}
	config := &ssh.ClientConfig{
		User:            host.SSHUser,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
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
	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("run: %w: %s", err, out.String())
	}
	return out.String(), nil
}
