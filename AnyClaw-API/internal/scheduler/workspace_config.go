package scheduler

import (
	"encoding/json"
	"fmt"
	"log"
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
		s = NormalizeAgentID(s)
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
// 若实例未绑定宿主机或读取失败，返回 (nil, nil) 表示跳过（不视为错误）。
// 实例停止时工作区文件仍可能存在，故不再要求 Status==running，以便打开编排页即可从 config.json 补全节点。
func (s *Scheduler) ReadWorkspaceConfigAgentSlugs(inst *db.Instance) ([]string, error) {
	if s == nil || inst == nil {
		return nil, nil
	}
	if inst.HostID == "" {
		return nil, nil
	}
	host, err := s.hosts.GetHost(inst.HostID)
	if err != nil || host == nil {
		return nil, nil
	}
	path := WorkspaceConfigPath(inst.ID)
	cmd := fmt.Sprintf("cat '%s'", shellEscapeSingleQuoted(path))
	out, err := runSSH(host, cmd)
	if err != nil {
		log.Printf("[collab] workspace config: instance %d cat %s: %v", inst.ID, path, err)
		return nil, nil
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	slugs, err := ParseAgentSlugsFromConfigJSON([]byte(out))
	if err != nil {
		log.Printf("[collab] workspace config: instance %d parse config.json: %v", inst.ID, err)
		return nil, nil
	}
	return slugs, nil
}
