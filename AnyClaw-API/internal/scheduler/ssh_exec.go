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
	signer, err := ssh.ParsePrivateKey([]byte(host.SSHKey))
	if err != nil {
		return "", fmt.Errorf("parse ssh key: %w", err)
	}
	config := &ssh.ClientConfig{
		User: host.SSHUser,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
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
