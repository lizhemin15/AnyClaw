package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port         int           `json:"port" env:"ANYCLAW_API_PORT"`
	DBDSN        string        `json:"db_dsn"`
	JWTSecret    string        `json:"jwt_secret"`
	APIURL       string        `json:"api_url"`
	DockerImage  string        `json:"docker_image"`
	DefaultModel string        `json:"default_model"` // deprecated
	Channels     []Channel     `json:"channels"`     // 用户添加的渠道，每个渠道可配置、启用、添加多个模型
	KeyPool      KeyPool       `json:"key_pool"`     // deprecated, migrate to channels
	InstanceMap  InstanceMap   `json:"instance_map"`
}

// Channel 渠道：用户添加，可配置、启用，每个渠道可添加多个模型
type Channel struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`    // 如 "OpenAI 主账号"
	APIKey  string        `json:"api_key"`
	APIBase string        `json:"api_base"`
	Enabled bool          `json:"enabled"`
	Models  []ModelEntry  `json:"models"` // 该渠道下的模型列表
}

type ModelEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`    // 如 gpt-4o
	Enabled bool   `json:"enabled"` // 全局仅一个模型可启用（新宠物默认）
}

// GetEnabledModel 返回当前启用的模型名
func (c *Config) GetEnabledModel() string {
	for _, ch := range c.Channels {
		for _, m := range ch.Models {
			if m.Enabled && m.Name != "" {
				return m.Name
			}
		}
	}
	if c.DefaultModel != "" {
		return c.DefaultModel
	}
	return ""
}

// FindChannelForModel 返回能提供该模型的已启用渠道
func (c *Config) FindChannelForModel(model string) (apiBase, apiKey string) {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, ch := range c.Channels {
		if !ch.Enabled || ch.APIKey == "" {
			continue
		}
		for _, m := range ch.Models {
			if strings.ToLower(strings.TrimSpace(m.Name)) == model {
				base := ch.APIBase
				if base == "" {
					base = "https://api.openai.com/v1"
				}
				return strings.TrimSuffix(base, "/"), ch.APIKey
			}
		}
	}
	// 无精确匹配时，按模型名推断渠道（gpt->openai风格, claude->anthropic风格）
	for _, ch := range c.Channels {
		if !ch.Enabled || ch.APIKey == "" {
			continue
		}
		base := ch.APIBase
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		chLower := strings.ToLower(ch.Name)
		if (strings.Contains(model, "gpt") || strings.Contains(model, "openai")) && (strings.Contains(chLower, "openai") || strings.Contains(chLower, "openrouter")) {
			return strings.TrimSuffix(base, "/"), ch.APIKey
		}
		if strings.Contains(model, "claude") && strings.Contains(chLower, "anthropic") {
			return strings.TrimSuffix(base, "/"), ch.APIKey
		}
	}
	// 最后尝试：任意有模型的已启用渠道
	for _, ch := range c.Channels {
		if ch.Enabled && ch.APIKey != "" && len(ch.Models) > 0 {
			base := ch.APIBase
			if base == "" {
				base = "https://api.openai.com/v1"
			}
			return strings.TrimSuffix(base, "/"), ch.APIKey
		}
	}
	return "", ""
}

type KeyPool struct {
	OpenAI    KeyEntry `json:"openai"`
	Anthropic KeyEntry `json:"anthropic"`
	OpenRouter KeyEntry `json:"openrouter"`
}

type KeyEntry struct {
	APIKey  string `json:"api_key"`
	APIBase string `json:"api_base"`
}

type InstanceMap struct {
	Tokens map[string]InstanceInfo `json:"tokens"`
}

type InstanceInfo struct {
	InstanceID string `json:"instance_id"`
	UserID    string `json:"user_id"`
}

func ConfigPath() string {
	p := os.Getenv("ANYCLAW_CONFIG_PATH")
	if p != "" {
		return p
	}
	return "/data/config.json"
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		Port:        8080,
		DBDSN:       "",
		DockerImage: "anyclaw/anyclaw",
	}
	if path == "" {
		path = ConfigPath()
	}
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, cfg) == nil {
			// ensure required defaults
			if cfg.DockerImage == "" {
				cfg.DockerImage = "anyclaw/anyclaw"
			}
		}
	}
	loadFromEnv(cfg)
	if cfg.InstanceMap.Tokens == nil {
		cfg.InstanceMap.Tokens = make(map[string]InstanceInfo)
	}
	if cfg.Channels == nil {
		cfg.Channels = []Channel{}
	}
	// 迁移：key_pool 或 model_list -> channels
	if len(cfg.Channels) == 0 {
		migrateToChannels(cfg)
	}
	// Env can override file
	if s := os.Getenv("ANYCLAW_INSTANCE_TOKENS"); s != "" {
		var m map[string]InstanceInfo
		if json.Unmarshal([]byte(s), &m) == nil {
			cfg.InstanceMap.Tokens = m
		}
	}
	return cfg, nil
}

type SaveConfig struct {
	DBDSN     string `json:"db_dsn"`
	JWTSecret string `json:"jwt_secret"`
}

func Save(path string, c *SaveConfig) error {
	if path == "" {
		path = ConfigPath()
	}
	dir := path
	for i := len(dir) - 1; i >= 0; i-- {
		if dir[i] == '/' || dir[i] == '\\' {
			dir = path[:i]
			break
		}
	}
	if dir != path {
		os.MkdirAll(dir, 0755)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// SaveAdminConfig saves channels to config file. Preserves other fields.
func SaveAdminConfig(path string, channels []Channel) error {
	if path == "" {
		path = ConfigPath()
	}
	dir := path
	for i := len(dir) - 1; i >= 0; i-- {
		if dir[i] == '/' || dir[i] == '\\' {
			dir = path[:i]
			break
		}
	}
	if dir != path {
		os.MkdirAll(dir, 0755)
	}
	var raw map[string]any
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &raw)
	}
	if raw == nil {
		raw = make(map[string]any)
	}
	raw["channels"] = channels
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func migrateToChannels(cfg *Config) {
	if cfg.KeyPool.OpenAI.APIKey != "" {
		base := cfg.KeyPool.OpenAI.APIBase
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		cfg.Channels = append(cfg.Channels, Channel{
			ID:      "openai",
			Name:    "OpenAI",
			APIKey:  cfg.KeyPool.OpenAI.APIKey,
			APIBase: base,
			Enabled: true,
			Models:  []ModelEntry{{ID: "gpt4o", Name: "gpt-4o", Enabled: true}},
		})
	}
	if cfg.KeyPool.Anthropic.APIKey != "" {
		base := cfg.KeyPool.Anthropic.APIBase
		if base == "" {
			base = "https://api.anthropic.com/v1"
		}
		cfg.Channels = append(cfg.Channels, Channel{
			ID:      "anthropic",
			Name:    "Anthropic Claude",
			APIKey:  cfg.KeyPool.Anthropic.APIKey,
			APIBase: base,
			Enabled: true,
			Models:  []ModelEntry{{ID: "claude", Name: "claude-3-5-sonnet", Enabled: false}},
		})
	}
	if cfg.KeyPool.OpenRouter.APIKey != "" {
		base := cfg.KeyPool.OpenRouter.APIBase
		if base == "" {
			base = "https://openrouter.ai/api/v1"
		}
		cfg.Channels = append(cfg.Channels, Channel{
			ID:      "openrouter",
			Name:    "OpenRouter",
			APIKey:  cfg.KeyPool.OpenRouter.APIKey,
			APIBase: base,
			Enabled: true,
			Models:  []ModelEntry{{ID: "auto", Name: "openrouter/auto", Enabled: false}},
		})
	}
}

// MaskAPIKey returns last 4 chars for display, empty if key empty.
func MaskAPIKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 4 {
		return "****"
	}
	return "****" + k[len(k)-4:]
}

func loadFromEnv(c *Config) {
	if v := os.Getenv("ANYCLAW_DB_DSN"); v != "" {
		c.DBDSN = v
	}
	if v := os.Getenv("ANYCLAW_JWT_SECRET"); v != "" {
		c.JWTSecret = v
	}
	if v := os.Getenv("ANYCLAW_API_URL"); v != "" {
		c.APIURL = v
	}
	if v := os.Getenv("ANYCLAW_DOCKER_IMAGE"); v != "" {
		c.DockerImage = v
	}
	if v := os.Getenv("ANYCLAW_API_PORT"); v != "" {
		var p int
		if _, err := fmt.Sscanf(v, "%d", &p); err == nil {
			c.Port = p
		}
	}
	if v := os.Getenv("ANYCLAW_KEY_OPENAI_API_KEY"); v != "" {
		c.KeyPool.OpenAI.APIKey = v
	}
	if v := os.Getenv("ANYCLAW_KEY_OPENAI_API_BASE"); v != "" {
		c.KeyPool.OpenAI.APIBase = v
	} else if c.KeyPool.OpenAI.APIBase == "" {
		c.KeyPool.OpenAI.APIBase = "https://api.openai.com/v1"
	}
	if v := os.Getenv("ANYCLAW_KEY_ANTHROPIC_API_KEY"); v != "" {
		c.KeyPool.Anthropic.APIKey = v
	}
	if v := os.Getenv("ANYCLAW_KEY_ANTHROPIC_API_BASE"); v != "" {
		c.KeyPool.Anthropic.APIBase = v
	} else if c.KeyPool.Anthropic.APIBase == "" {
		c.KeyPool.Anthropic.APIBase = "https://api.anthropic.com/v1"
	}
	if v := os.Getenv("ANYCLAW_KEY_OPENROUTER_API_KEY"); v != "" {
		c.KeyPool.OpenRouter.APIKey = v
	}
	if v := os.Getenv("ANYCLAW_KEY_OPENROUTER_API_BASE"); v != "" {
		c.KeyPool.OpenRouter.APIBase = v
	} else if c.KeyPool.OpenRouter.APIBase == "" {
		c.KeyPool.OpenRouter.APIBase = "https://openrouter.ai/api/v1"
	}
}
