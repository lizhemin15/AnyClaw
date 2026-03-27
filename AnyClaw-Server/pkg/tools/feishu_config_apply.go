package tools

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/anyclaw/anyclaw-server/pkg/channels/feishu"
	"github.com/anyclaw/anyclaw-server/pkg/config"
)

var errFeishuEmptyCreds = errors.New("app_id and app_secret must be non-empty")

// persistFeishuCredentials writes Feishu app_id/app_secret, optionally appends allow_from
// entries, saves config, records binding pending notification, and schedules gateway restart.
func persistFeishuCredentials(ctx context.Context, appID, appSecret string, extraAllowFrom []string) error {
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	if appID == "" || appSecret == "" {
		return errFeishuEmptyCreds
	}

	configPath := getConfigPath()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return err
	}

	cfg.Channels.Feishu.Enabled = true
	cfg.Channels.Feishu.AppID = appID
	cfg.Channels.Feishu.AppSecret = appSecret

	af := []string(cfg.Channels.Feishu.AllowFrom)
	for _, id := range extraAllowFrom {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !slices.Contains(af, id) {
			af = append(af, id)
		}
	}
	cfg.Channels.Feishu.AllowFrom = af

	if err := config.SaveConfig(configPath, cfg); err != nil {
		return err
	}

	if ch, chatID := ToolChannel(ctx), ToolChatID(ctx); ch != "" && chatID != "" {
		_ = feishu.WriteBindingPending(ch, chatID)
	}
	scheduleRestart()
	return nil
}

func getConfigPath() string {
	return config.DefaultConfigPath()
}
