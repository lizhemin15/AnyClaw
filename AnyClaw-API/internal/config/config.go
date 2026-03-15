package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port         int            `json:"port" env:"ANYCLAW_API_PORT"`
	DBDSN        string         `json:"db_dsn"`
	JWTSecret    string         `json:"jwt_secret"`
	APIURL       string         `json:"api_url"`
	DockerImage  string         `json:"docker_image"`
	DefaultModel string         `json:"default_model"` // deprecated
	Channels     []Channel      `json:"channels"`     // 用户添加的渠道，每个渠道可配置、启用、添加多个模型
	KeyPool      KeyPool        `json:"key_pool"`     // deprecated, migrate to channels
	InstanceMap  InstanceMap    `json:"instance_map"`
	SMTP         *SMTPConfig    `json:"smtp,omitempty"` // 注册验证码邮件
	Payment      *PaymentConfig `json:"payment,omitempty"`
	Energy       *EnergyConfig  `json:"energy,omitempty"` // 金币/活力经济参数，即时生效
	Container    *ContainerConfig `json:"container,omitempty"` // 员工容器配置
	COS          *COSConfig      `json:"cos,omitempty"`       // 腾讯云 COS 对象存储，用于媒体文件
}

// COSConfig 腾讯云 COS 配置，用于上传 AI 发送的图片/音视频/文件
type COSConfig struct {
	Enabled    bool   `json:"enabled"`
	SecretID   string `json:"secret_id"`
	SecretKey  string `json:"secret_key"`
	Bucket     string `json:"bucket"`      // 存储桶名称，如 mybucket-1234567890
	Region     string `json:"region"`      // 地域，如 ap-guangzhou
	Domain     string `json:"domain"`      // 自定义域名，空则用默认 cos.<bucket>.cos.<region>.myqcloud.com
	PathPrefix string `json:"path_prefix"` // 对象键前缀，如 media/
}

// ContainerConfig 宠物容器配置
type ContainerConfig struct {
	WorkspaceSizeGB int `json:"workspace_size_gb"` // 每个实例工作区存储上限(GB)，0 表示不限制
}

// GetWorkspaceSizeGB 返回工作区存储上限(GB)，0 表示不限制
func GetWorkspaceSizeGB(cfg *Config) int {
	if cfg == nil || cfg.Container == nil {
		return 0
	}
	if cfg.Container.WorkspaceSizeGB < 0 {
		return 0
	}
	return cfg.Container.WorkspaceSizeGB
}

// EnergyConfig 金币经济配置，全部即时生效
type EnergyConfig struct {
	TokensPerEnergy       int `json:"tokens_per_energy"`       // 每 N token 消耗 1 活力，默认 1000
	AdoptCost             int `json:"adopt_cost"`              // 领养宠物消耗金币，默认 100
	DailyConsume          int `json:"daily_consume"`            // 每只宠物每日消耗活力，默认 10
	MinEnergyForTask      int `json:"min_energy_for_task"`     // 低于此值无法对话，默认 5
	ZeroDaysToDelete      int `json:"zero_days_to_delete"`     // 连续无活力天数后永久消失，默认 3
	InviteReward          int `json:"invite_reward"`           // 邀请奖励（双方各得），默认 50
	NewUserEnergy         int `json:"new_user_energy"`          // 新用户初始金币，默认 0
	DailyLoginBonus       int `json:"daily_login_bonus"`       // 每天首次登录赠送金币，默认 10
	InviteCommissionRate  int `json:"invite_commission_rate"`   // 受邀用户充值时的邀请人返利比例(0-100)，默认 5
	MonthlySubscriptionCost int `json:"monthly_subscription_cost"` // 单只宠物包月价格（金币），默认 50，0 表示禁用包月
}

// GetEnergyDefaults 返回默认值
func GetEnergyDefaults() EnergyConfig {
	return EnergyConfig{
		TokensPerEnergy:        1000,
		AdoptCost:              100,
		DailyConsume:           10,
		MinEnergyForTask:       5,
		ZeroDaysToDelete:       3,
		InviteReward:           50,
		NewUserEnergy:          0,
		DailyLoginBonus:        10,
		InviteCommissionRate:   5,
		MonthlySubscriptionCost: 50,
	}
}

// GetEnergyConfig 从 cfg 获取能量配置，带默认值
func GetEnergyConfig(cfg *Config) EnergyConfig {
	def := GetEnergyDefaults()
	if cfg == nil || cfg.Energy == nil {
		return def
	}
	e := *cfg.Energy
	if e.TokensPerEnergy <= 0 {
		e.TokensPerEnergy = def.TokensPerEnergy
	}
	if e.AdoptCost <= 0 {
		e.AdoptCost = def.AdoptCost
	}
	if e.DailyConsume < 0 {
		e.DailyConsume = def.DailyConsume
	}
	if e.MinEnergyForTask < 0 {
		e.MinEnergyForTask = def.MinEnergyForTask
	}
	if e.ZeroDaysToDelete <= 0 {
		e.ZeroDaysToDelete = def.ZeroDaysToDelete
	}
	if e.InviteReward < 0 {
		e.InviteReward = def.InviteReward
	}
	if e.NewUserEnergy < 0 {
		e.NewUserEnergy = def.NewUserEnergy
	}
	if e.DailyLoginBonus < 0 {
		e.DailyLoginBonus = def.DailyLoginBonus
	}
	if e.InviteCommissionRate < 0 || e.InviteCommissionRate > 100 {
		e.InviteCommissionRate = def.InviteCommissionRate
	}
	if e.MonthlySubscriptionCost < 0 {
		e.MonthlySubscriptionCost = def.MonthlySubscriptionCost
	}
	return e
}

// PaymentConfig 支付配置：YunGouOS（微信/支付宝）、充值档位
type PaymentConfig struct {
	Yungouos *YungouosConfig `json:"yungouos,omitempty"`
	Plans    []PaymentPlan   `json:"plans,omitempty"`
}

// YungouosConfig YunGouOS 云购OS 配置（个人可开通，支持微信/支付宝扫码）
type YungouosConfig struct {
	Wechat *YungouosChannel `json:"wechat,omitempty"`
	Alipay *YungouosChannel `json:"alipay,omitempty"`
}

// YungouosChannel YunGouOS 单渠道配置
type YungouosChannel struct {
	Enabled bool   `json:"enabled"`
	MchID   string `json:"mch_id"`  // 商户号，登录 yungouos.com 商户管理获取
	Key     string `json:"key"`    // 支付密钥
}

// PaymentPlan 充值档位
type PaymentPlan struct {
	ID       string `json:"id"`
	Name     string `json:"name"`     // 档位名称
	Benefits string `json:"benefits"`  // 权益介绍
	Energy   int    `json:"energy"`    // 金币数量
	PriceCny int    `json:"price_cny"` // 价格（分）
	Sort     int    `json:"sort"`
}

// SMTPConfig 邮件服务配置
type SMTPConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"` // 587 or 465
	User string `json:"user"`
	Pass string `json:"pass"`
	From string `json:"from"` // 发件人，空则用 User
}

// Channel 渠道：用户添加，可配置、启用，每个渠道可添加多个模型
type Channel struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`    // 如 "OpenAI 主账号"
	APIKey           string       `json:"api_key"`
	APIBase          string       `json:"api_base"`
	Enabled          bool         `json:"enabled"`
	Models           []ModelEntry `json:"models"`  // 该渠道下的模型列表
	DailyTokensLimit int64        `json:"daily_tokens_limit"` // 日 tokens 上限，0 表示不限制
	QPSLimit         float64      `json:"qps_limit"`          // 每秒请求数上限，0 表示不限制
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

// ChannelEndpoint 渠道端点，用于调度
type ChannelEndpoint struct {
	ChannelID         string
	ChannelName       string  // 渠道展示名，如"OpenAI 主账号"
	APIBase           string
	APIKey            string
	DailyTokensLimit  int64   // 0=不限制
	QPSLimit          float64 // 0=不限制
}

// FindChannelsForModel 返回能提供该模型的所有已启用渠道（用于负载均衡）
func (c *Config) FindChannelsForModel(model string) []ChannelEndpoint {
	model = strings.ToLower(strings.TrimSpace(model))
	var out []ChannelEndpoint
	seen := make(map[string]bool)
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
				key := ch.ID + "|" + base
				if !seen[key] {
					seen[key] = true
				out = append(out, ChannelEndpoint{ChannelID: ch.ID, ChannelName: ch.Name, APIBase: strings.TrimSuffix(base, "/"), APIKey: ch.APIKey, DailyTokensLimit: ch.DailyTokensLimit, QPSLimit: ch.QPSLimit})
			}
			break
			}
		}
	}
	if len(out) > 0 {
		return out
	}
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
			key := ch.ID + "|" + base
			if !seen[key] {
				seen[key] = true
				out = append(out, ChannelEndpoint{ChannelID: ch.ID, ChannelName: ch.Name, APIBase: strings.TrimSuffix(base, "/"), APIKey: ch.APIKey, DailyTokensLimit: ch.DailyTokensLimit, QPSLimit: ch.QPSLimit})
			}
		}
		if strings.Contains(model, "claude") && strings.Contains(chLower, "anthropic") {
			key := ch.ID + "|" + base
			if !seen[key] {
				seen[key] = true
				out = append(out, ChannelEndpoint{ChannelID: ch.ID, ChannelName: ch.Name, APIBase: strings.TrimSuffix(base, "/"), APIKey: ch.APIKey, DailyTokensLimit: ch.DailyTokensLimit, QPSLimit: ch.QPSLimit})
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, ch := range c.Channels {
		if ch.Enabled && ch.APIKey != "" && len(ch.Models) > 0 {
			base := ch.APIBase
			if base == "" {
				base = "https://api.openai.com/v1"
			}
			key := ch.ID + "|" + base
			if !seen[key] {
				seen[key] = true
				out = append(out, ChannelEndpoint{ChannelID: ch.ID, ChannelName: ch.Name, APIBase: strings.TrimSuffix(base, "/"), APIKey: ch.APIKey, DailyTokensLimit: ch.DailyTokensLimit, QPSLimit: ch.QPSLimit})
			}
		}
	}
	return out
}

// FindChannelForModel 返回能提供该模型的已启用渠道（兼容旧逻辑，取第一个）
func (c *Config) FindChannelForModel(model string) (apiBase, apiKey string) {
	list := c.FindChannelsForModel(model)
	if len(list) == 0 {
		return "", ""
	}
	return list[0].APIBase, list[0].APIKey
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

// LoadFromDB 可选：当文件无 channels 时从 DB 加载（用于 Sealos/K8s 等 /data 不持久化）
var LoadFromDB func() ([]byte, error)

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
	// 优先从 DB 加载 admin 配置（channels/smtp/payment/energy），DB 为唯一数据源
	if LoadFromDB != nil {
		if b, err := LoadFromDB(); err == nil && len(b) > 0 {
			var dbCfg struct {
				Channels   []Channel          `json:"channels"`
				SMTP       *SMTPConfig        `json:"smtp"`
				Payment    *PaymentConfig     `json:"payment"`
				Energy     *EnergyConfig      `json:"energy"`
				Container  *ContainerConfig   `json:"container"`
				COS        *COSConfig         `json:"cos"`
				APIURL     string             `json:"api_url"`
			}
			if json.Unmarshal(b, &dbCfg) == nil {
				if len(dbCfg.Channels) > 0 {
					cfg.Channels = dbCfg.Channels
				}
				if dbCfg.SMTP != nil {
					cfg.SMTP = dbCfg.SMTP
				}
				if dbCfg.Payment != nil {
					cfg.Payment = dbCfg.Payment
				}
				if dbCfg.Energy != nil {
					cfg.Energy = dbCfg.Energy
				}
				if dbCfg.Container != nil {
					cfg.Container = dbCfg.Container
				}
				if dbCfg.COS != nil {
					cfg.COS = dbCfg.COS
				}
				if dbCfg.APIURL != "" {
					cfg.APIURL = strings.TrimSpace(dbCfg.APIURL)
				}
			}
		}
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

// SaveAdminConfig saves channels, smtp, payment, energy to config file. Preserves other fields.
func SaveAdminConfig(path string, channels []Channel, smtp *SMTPConfig, payment *PaymentConfig, energy *EnergyConfig) error {
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
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	var raw map[string]any
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &raw)
	}
	if raw == nil {
		raw = make(map[string]any)
	}
	raw["channels"] = channels
	if smtp != nil {
		raw["smtp"] = smtp
	}
	if payment != nil {
		raw["payment"] = payment
	}
	if energy != nil {
		raw["energy"] = energy
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
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
