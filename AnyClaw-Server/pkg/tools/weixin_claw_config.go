package tools

import (
	"fmt"
	"slices"
	"strings"

	"github.com/anyclaw/anyclaw-server/pkg/config"
)

// persistWeixinClawBindingToConfig 将微信 ilink 凭证写入 config.json（与飞书同级），并启用 weixin_claw、合并 allow_from。
func persistWeixinClawBindingToConfig(accountID, token, baseURL, userID string) error {
	accountID = strings.TrimSpace(accountID)
	token = strings.TrimSpace(token)
	if accountID == "" || token == "" {
		return fmt.Errorf("account_id 与 token 不能为空")
	}
	configPath := getConfigPath()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return err
	}
	cfg.Channels.WeixinClaw.Enabled = true
	bu := strings.TrimSpace(baseURL)
	if bu == "" {
		bu = "https://ilinkai.weixin.qq.com"
	}
	uid := strings.TrimSpace(userID)
	found := false
	for i := range cfg.Channels.WeixinClaw.Accounts {
		if strings.TrimSpace(cfg.Channels.WeixinClaw.Accounts[i].AccountID) == accountID {
			cfg.Channels.WeixinClaw.Accounts[i].Token = token
			cfg.Channels.WeixinClaw.Accounts[i].BaseURL = bu
			cfg.Channels.WeixinClaw.Accounts[i].UserID = uid
			found = true
			break
		}
	}
	if !found {
		cfg.Channels.WeixinClaw.Accounts = append(cfg.Channels.WeixinClaw.Accounts, config.WeixinClawAccount{
			AccountID: accountID,
			Token:     token,
			BaseURL:   bu,
			UserID:    uid,
		})
	}
	if uid != "" {
		af := []string(cfg.Channels.WeixinClaw.AllowFrom)
		if !slices.Contains(af, uid) {
			af = append(af, uid)
		}
		cfg.Channels.WeixinClaw.AllowFrom = af
	}
	return config.SaveConfig(configPath, cfg)
}
