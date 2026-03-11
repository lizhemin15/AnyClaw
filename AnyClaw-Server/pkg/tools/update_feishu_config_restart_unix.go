//go:build unix

package tools

import (
	"os"
	"syscall"
	"time"

	"github.com/anyclaw/anyclaw-server/pkg/logger"
)

// scheduleRestart schedules a process restart after a short delay so the tool
// response can be sent to the user before the process is replaced.
func scheduleRestart() {
	go func() {
		time.Sleep(3 * time.Second)
		execPath, err := os.Executable()
		if err != nil {
			logger.ErrorCF("feishu_config", "Failed to get executable path for restart", map[string]any{"error": err.Error()})
			return
		}
		args := append([]string{execPath}, os.Args[1:]...)
		logger.InfoC("feishu_config", "Restarting AnyClaw to apply Feishu config")
		if err := syscall.Exec(execPath, args, os.Environ()); err != nil {
			logger.ErrorCF("feishu_config", "Failed to restart process", map[string]any{"error": err.Error()})
		}
	}()
}
