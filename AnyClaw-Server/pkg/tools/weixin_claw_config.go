package tools

import (
	"slices"
	"strings"

	"github.com/anyclaw/anyclaw-server/pkg/config"
)

// enableWeixinClawInConfig turns on channels.weixin_claw and optionally appends the bound user's ilink id to allow_from.
func enableWeixinClawInConfig(allowUserID string) error {
	configPath := getConfigPath()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return err
	}
	cfg.Channels.WeixinClaw.Enabled = true
	if id := strings.TrimSpace(allowUserID); id != "" {
		af := []string(cfg.Channels.WeixinClaw.AllowFrom)
		if !slices.Contains(af, id) {
			af = append(af, id)
		}
		cfg.Channels.WeixinClaw.AllowFrom = af
	}
	return config.SaveConfig(configPath, cfg)
}
