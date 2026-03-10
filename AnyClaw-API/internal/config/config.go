package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Port        int    `json:"port" env:"ANYCLAW_API_PORT"`
	DBDSN       string `json:"db_dsn"`
	JWTSecret   string `json:"jwt_secret"`
	APIURL      string      `json:"api_url"`       // e.g. http://localhost:8080 for Docker containers
	DockerImage string      `json:"docker_image"`  // openclaw/openclaw
	DefaultModel string    `json:"default_model"`  // 宠物默认使用的模型，如 gpt-4o
	KeyPool     KeyPool     `json:"key_pool"`
	InstanceMap InstanceMap `json:"instance_map"`  // legacy: tokens from config (merged with DB)
}

type KeyPool struct {
	OpenAI    KeyEntry `json:"openai"`
	Anthropic KeyEntry `json:"anthropic"`
	OpenRouter KeyEntry `json:"openrouter"`
}

type KeyEntry struct {
	APIKey  string `json:"api_key" env:"ANYCLAW_KEY_OPENAI_API_KEY"`
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
		DockerImage: "openclaw/openclaw",
	}
	if path == "" {
		path = ConfigPath()
	}
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, cfg) == nil {
			// ensure required defaults
			if cfg.DockerImage == "" {
				cfg.DockerImage = "openclaw/openclaw"
			}
		}
	}
	loadFromEnv(cfg)
	if cfg.InstanceMap.Tokens == nil {
		cfg.InstanceMap.Tokens = make(map[string]InstanceInfo)
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

// SaveKeyPool merges KeyPool and optionally default_model into config file and writes. Preserves other fields.
func SaveKeyPool(path string, pool KeyPool, defaultModel string) error {
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
	raw["key_pool"] = pool
	raw["default_model"] = defaultModel
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
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
