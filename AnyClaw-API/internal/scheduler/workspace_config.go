package scheduler

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/db"
)

// ParseAgentSlugsFromConfigJSON 解析与 anyclaw-server 一致的 config.json，提取 agents.list[].id。
// list 为空时返回 ["main"]，与无 agents.list 时隐式 main 一致。
func ParseAgentSlugsFromConfigJSON(data []byte) ([]string, error) {
	var cfg struct {
		Agents struct {
			List []struct {
				ID string `json:"id"`
			} `json:"list"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.Agents.List) == 0 {
		return []string{"main"}, nil
	}
	seen := make(map[string]struct{})
	var slugs []string
	for _, item := range cfg.Agents.List {
		s := strings.TrimSpace(item.ID)
		if s == "" {
			s = "main"
		}
		s = strings.ToLower(s)
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		slugs = append(slugs, s)
	}
	if len(slugs) == 0 {
		return []string{"main"}, nil
	}
	return slugs, nil
}

// WorkspaceConfigPath 与 buildDockerRunCmd 挂载的工作区路径一致。
func WorkspaceConfigPath(instanceID int64) string {
	return fmt.Sprintf("/var/lib/anyclaw/ws-%d/config.json", instanceID)
}

// ReadWorkspaceConfigAgentSlugs 在实例宿主机上读取工作区 config.json 中的 agents.list id（需 SSH）。
// 若实例未绑定宿主机、非运行中、或读取失败，返回 (nil, nil) 表示跳过（不视为错误）。
func (s *Scheduler) ReadWorkspaceConfigAgentSlugs(inst *db.Instance) ([]string, error) {
	if s == nil || inst == nil {
		return nil, nil
	}
	if inst.HostID == "" || inst.Status != "running" {
		return nil, nil
	}
	host, err := s.hosts.GetHost(inst.HostID)
	if err != nil || host == nil {
		return nil, nil
	}
	path := WorkspaceConfigPath(inst.ID)
	cmd := fmt.Sprintf("cat '%s'", shellEscapeSingleQuoted(path))
	out, err := runSSH(host, cmd)
	if err != nil || strings.TrimSpace(out) == "" {
		return nil, nil
	}
	slugs, err := ParseAgentSlugsFromConfigJSON([]byte(out))
	if err != nil {
		return nil, nil
	}
	return slugs, nil
}
