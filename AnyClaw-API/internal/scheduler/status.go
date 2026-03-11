package scheduler

import (
	"github.com/anyclaw/anyclaw-api/internal/db"
)

// HostChecker verifies SSH connectivity and Docker availability.
type HostChecker struct{}

func (HostChecker) CheckHost(host *db.Host) (string, error) {
	_, err := runSSH(host, "docker ps >/dev/null 2>&1")
	if err != nil {
		return "error", err
	}
	return "online", nil
}

// RunCommand 在宿主机上执行 SSH 命令，返回输出
func (HostChecker) RunCommand(host *db.Host, cmd string) (string, error) {
	return runSSH(host, cmd)
}
