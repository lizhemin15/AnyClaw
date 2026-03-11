package tools

import (
	"context"
	"os"
	"path/filepath"

	"github.com/anyclaw/anyclaw-server/pkg/config"
)

// UpdateFeishuConfigTool updates the Feishu channel config (app_id, app_secret) in config.json.
// Used when users provide credentials via chat (e.g. "绑定飞书，app_id 是 cli_xxx，app_secret 是 xxx").
type UpdateFeishuConfigTool struct{}

func NewUpdateFeishuConfigTool() *UpdateFeishuConfigTool {
	return &UpdateFeishuConfigTool{}
}

func (t *UpdateFeishuConfigTool) Name() string {
	return "update_feishu_config"
}

func (t *UpdateFeishuConfigTool) Description() string {
	return "Update Feishu channel config (app_id, app_secret) in config.json. Call this when the user provides Feishu credentials in natural language to bind Feishu. After updating, AnyClaw will automatically restart to apply the Feishu channel."
}

func (t *UpdateFeishuConfigTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"app_id": map[string]any{
				"type":        "string",
				"description": "Feishu App ID (starts with cli_)",
			},
			"app_secret": map[string]any{
				"type":        "string",
				"description": "Feishu App Secret",
			},
		},
		"required": []string{"app_id", "app_secret"},
	}
}

func (t *UpdateFeishuConfigTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	appID, ok := args["app_id"].(string)
	if !ok || appID == "" {
		return ErrorResult("app_id is required and must be non-empty")
	}
	appSecret, ok := args["app_secret"].(string)
	if !ok || appSecret == "" {
		return ErrorResult("app_secret is required and must be non-empty")
	}

	configPath := getConfigPath()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return ErrorResult("failed to load config: " + err.Error())
	}

	cfg.Channels.Feishu.Enabled = true
	cfg.Channels.Feishu.AppID = appID
	cfg.Channels.Feishu.AppSecret = appSecret

	if err := config.SaveConfig(configPath, cfg); err != nil {
		return ErrorResult("failed to save config: " + err.Error())
	}

	scheduleRestart()
	return SilentResult("Feishu config updated. AnyClaw will restart in a few seconds to apply the Feishu channel.")
}

func getConfigPath() string {
	if p := os.Getenv("ANYCLAW_CONFIG"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".anyclaw", "config.json")
}
