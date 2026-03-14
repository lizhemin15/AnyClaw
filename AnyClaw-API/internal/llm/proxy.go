package llm

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/db"
)

// TokenResolver resolves a Bearer token to instance and user IDs.
type TokenResolver interface {
	ResolveToken(token string) (instanceID, userID string, ok bool)
}

// UsageRecord is logged for each LLM call.
type UsageRecord struct {
	InstanceID string `json:"instance_id"`
	UserID     string `json:"user_id"`
	Model      string `json:"model"`
	Provider   string `json:"provider"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type Proxy struct {
	configPath string
	resolver   TokenResolver
	db         *db.DB
	client     *http.Client
	scheduler  *ModelScheduler
	mu         sync.RWMutex
}

func New(configPath string, resolver TokenResolver, database *db.DB) *Proxy {
	return &Proxy{
		configPath: configPath,
		resolver:   resolver,
		db:         database,
		scheduler:  NewModelScheduler(database),
		client: &http.Client{
			Timeout: 300 * time.Second, // 5 分钟，应对慢速 LLM 或网络延迟
		},
	}
}

// StartKeepAlive 启动保活协程，定期向各渠道发送最小请求
func (p *Proxy) StartKeepAlive(interval time.Duration) {
	ka := NewKeepAlive(p.configPath, interval)
	ka.Start()
}

func (p *Proxy) loadConfig() (*config.Config, error) {
	return config.Load(p.configPath)
}

// HandleChatCompletions proxies the request to the appropriate provider.
func (p *Proxy) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	token := extractBearer(r)
	if token == "" {
		http.Error(w, `{"error":{"message":"missing authorization"}}`, http.StatusUnauthorized)
		return
	}

	instanceID := r.Header.Get("X-Instance-ID")
	userID := ""

	// Resolve token: DB first, then config (legacy)
	if p.resolver != nil {
		if id, uid, ok := p.resolver.ResolveToken(token); ok {
			instanceID = id
			userID = uid
		}
	}
	if instanceID == "" {
		if cfg, err := p.loadConfig(); err == nil {
			if info, ok := cfg.InstanceMap.Tokens[token]; ok {
				instanceID = info.InstanceID
				userID = info.UserID
			}
		}
	}
	if instanceID == "" {
		http.Error(w, `{"error":{"message":"invalid token"}}`, http.StatusUnauthorized)
		return
	}
	if userID == "" && p.db != nil {
		instID, _ := strconv.ParseInt(instanceID, 10, 64)
		if inst, err := p.db.GetInstanceByID(instID); err == nil && inst != nil {
			userID = strconv.FormatInt(inst.UserID, 10)
		}
	}

	// 检查用户金币（对话消耗用户金币；包月实例本月不消耗，可豁免）
	if p.db != nil && userID != "" {
		instID, _ := strconv.ParseInt(instanceID, 10, 64)
		subscribed, _ := p.db.IsInstanceSubscribed(instID)
		if !subscribed {
			cfg, _ := config.Load(p.configPath)
			minCoins := config.GetEnergyConfig(cfg).MinEnergyForTask
			if minCoins < 1 {
				minCoins = 1
			}
			uid, _ := strconv.ParseInt(userID, 10, 64)
			u, err := p.db.GetUserByID(uid)
			if err == nil && u != nil && u.Energy < minCoins {
				http.Error(w, `{"error":{"message":"金币不足，无法完成对话（需至少 `+strconv.Itoa(minCoins)+` 金币）"}}`, http.StatusPaymentRequired)
				return
			}
		}
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":{"message":"bad request"}}`, http.StatusBadRequest)
		return
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":{"message":"invalid json"}}`, http.StatusBadRequest)
		return
	}

	model, _ := req["model"].(string)
	cfg, cfgErr := p.loadConfig()
	if cfgErr != nil {
		http.Error(w, `{"error":{"message":"config error"}}`, http.StatusInternalServerError)
		return
	}
	if model == "" {
		model = cfg.GetEnabledModel()
	}
	if model == "" {
		model = "gpt-4o"
	}
	req["model"] = model
	candidates := cfg.FindChannelsForModel(model)
	if len(candidates) == 0 {
		log.Printf("[llm] no channel for model %q", model)
		http.Error(w, `{"error":{"message":"no provider configured for model"}}`, http.StatusServiceUnavailable)
		return
	}

	bodyBytes, _ := json.Marshal(req)

	// 最多尝试 min(len(candidates), 3) 次；每次 5xx 后换下一个渠道
	type attempt struct {
		ep         config.ChannelEndpoint
		statusCode int
		respHeader http.Header
		respBody   []byte
	}
	maxTries := len(candidates)
	if maxTries > 3 {
		maxTries = 3
	}
	var final *attempt
	for try := 0; try < maxTries; try++ {
		ep, ok := p.scheduler.Pick(model, candidates)
		if !ok {
			break
		}
		reqURL := strings.TrimSuffix(ep.APIBase, "/") + "/chat/completions"
		proxyReq, err := http.NewRequestWithContext(r.Context(), "POST", reqURL, bytes.NewReader(bodyBytes))
		if err != nil {
			p.scheduler.Done(ep)
			break
		}
		proxyReq.Header.Set("Content-Type", "application/json")
		proxyReq.Header.Set("Authorization", "Bearer "+ep.APIKey)
		if u, err := url.Parse(reqURL); err == nil && u.Host != "" {
			host := u.Hostname()
			if pt := u.Port(); pt != "" && pt != "443" && pt != "80" {
				host = u.Host
			}
			proxyReq.Host = host
		}
		resp, err := p.client.Do(proxyReq)
		p.scheduler.Done(ep)
		if err != nil {
			log.Printf("[llm] upstream error (try %d): channel=%s err=%v", try+1, ep.ChannelName, err)
			p.scheduler.RecordFailure(ep, CooldownTransient)
			continue
		}
		rb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			dur := cooldownFor(resp.StatusCode, rb)
			log.Printf("[llm] upstream error (try %d): channel=%s status=%d cooldown=%v body=%s",
				try+1, ep.ChannelName, resp.StatusCode, dur, truncate(string(rb), 200))
			p.scheduler.RecordFailure(ep, dur)
			if try < maxTries-1 {
				continue
			}
		}
		final = &attempt{ep: ep, statusCode: resp.StatusCode, respHeader: resp.Header, respBody: rb}
		break
	}

	if final == nil {
		http.Error(w, `{"error":{"message":"all upstream channels failed"}}`, http.StatusBadGateway)
		return
	}

	for k, v := range final.respHeader {
		if strings.ToLower(k) == "content-type" || strings.ToLower(k) == "content-length" {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
	}
	w.WriteHeader(final.statusCode)
	w.Write(final.respBody)

	respBody := final.respBody
	channelName := final.ep.ChannelName
	apiBase := final.ep.APIBase

	if final.statusCode != http.StatusOK {
		log.Printf("[llm] upstream non-200: channel=%s status=%d body=%s", channelName, final.statusCode, string(respBody))
	}

	if final.statusCode == http.StatusOK {
		// 对话成功说明实例在运行，若 DB 误标为 error 则纠正
		if p.db != nil && instanceID != "" {
			instID, _ := strconv.ParseInt(instanceID, 10, 64)
			if inst, err := p.db.GetInstanceByID(instID); err == nil && inst != nil && inst.Status == "error" {
				_ = p.db.UpdateInstanceStatus(instID, "running")
			}
		}
		promptTokens, completionTokens := parseUsageFromResponse(respBody)
		cost := 0
		if p.db != nil && userID != "" {
			instID, _ := strconv.ParseInt(instanceID, 10, 64)
			subscribed, _ := p.db.IsInstanceSubscribed(instID)
			if !subscribed {
				cfg, _ := config.Load(p.configPath)
				tokensPerEnergy := config.GetEnergyConfig(cfg).TokensPerEnergy
				cost = energyFromTokens(promptTokens, completionTokens, tokensPerEnergy)
				uid, _ := strconv.ParseInt(userID, 10, 64)
				if ok, _ := p.db.DeductUserEnergy(uid, cost); !ok {
					log.Printf("[llm] deduct user %d coins %d failed (insufficient balance?)", uid, cost)
				}
			}
		}
		provider := channelName
		if provider == "" {
			provider = apiBase
		}
		p.logUsage(instanceID, userID, model, provider, promptTokens, completionTokens, cost)
	}
}


func (p *Proxy) logUsage(instanceID, userID, model, provider string, promptTokens, completionTokens, coinsCost int) {
	rec := UsageRecord{
		InstanceID:       instanceID,
		UserID:           userID,
		Model:            model,
		Provider:         provider,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
	}
	log.Printf("[usage] %+v coins_cost=%d", rec, coinsCost)
	if p.db != nil {
		_ = p.db.InsertUsage(instanceID, userID, model, provider, promptTokens, completionTokens, coinsCost)
	}
}

// parseUsageFromResponse extracts prompt_tokens and completion_tokens from LLM response.
// Supports both JSON (single object) and SSE streaming (last chunk with usage).
func parseUsageFromResponse(respBody []byte) (promptTokens, completionTokens int) {
	// Try non-streaming JSON first
	var root struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &root); err == nil && (root.Usage.PromptTokens > 0 || root.Usage.CompletionTokens > 0 || root.Usage.TotalTokens > 0) {
		if root.Usage.TotalTokens > 0 && root.Usage.PromptTokens == 0 && root.Usage.CompletionTokens == 0 {
			return root.Usage.TotalTokens, 0
		}
		return root.Usage.PromptTokens, root.Usage.CompletionTokens
	}
	// Try SSE streaming: find last "data: {...}" chunk with usage
	lines := strings.Split(string(respBody), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
			continue
		}
		jsonPart := strings.TrimPrefix(line, "data: ")
		var chunk struct {
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(jsonPart), &chunk); err != nil {
			continue
		}
		if chunk.Usage.TotalTokens > 0 || chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			if chunk.Usage.TotalTokens > 0 && chunk.Usage.PromptTokens == 0 && chunk.Usage.CompletionTokens == 0 {
				return chunk.Usage.TotalTokens, 0
			}
			return chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens
		}
	}
	return 0, 0
}

// energyFromTokens returns energy cost: ceil(total_tokens / tokensPerEnergy), minimum 1.
func energyFromTokens(promptTokens, completionTokens int, tokensPerEnergy int) int {
	total := promptTokens + completionTokens
	if total <= 0 {
		return 1
	}
	if tokensPerEnergy <= 0 {
		tokensPerEnergy = 1000
	}
	cost := int(math.Ceil(float64(total) / float64(tokensPerEnergy)))
	if cost < 1 {
		cost = 1
	}
	return cost
}

func extractBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

// quotaPatterns 是上游响应体中表示"配额用尽 / 余额不足"的特征字符串（全小写）。
// 匹配到任意一项时渠道将被冷却整天，而非短暂 60 秒。
var quotaPatterns = []string{
	"quota",
	"rate_limit_exceeded",
	"appidnoautherror",   // 讯飞 AppId 无权限 / 超量
	"tokens_per_day",
	"daily_limit",
	"insufficient_quota",
	"insufficient_balance",
	"no balance",
	"balance insufficient",
	"billing",
	"11200",             // one-api 配额超限 code
	"exceeded your current quota",
	"you exceeded",
	"credit",
}

// cooldownFor 根据 HTTP 状态码和响应体判断合适的冷却时长。
// 429 或配额用尽类错误返回 CooldownDailyLimit；一般 5xx 返回 CooldownTransient。
func cooldownFor(statusCode int, body []byte) time.Duration {
	if statusCode == http.StatusTooManyRequests {
		return CooldownDailyLimit
	}
	lower := strings.ToLower(string(body))
	for _, p := range quotaPatterns {
		if strings.Contains(lower, p) {
			return CooldownDailyLimit
		}
	}
	return CooldownTransient
}

// truncate 截断字符串到 maxLen 个字符，用于日志输出。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}
