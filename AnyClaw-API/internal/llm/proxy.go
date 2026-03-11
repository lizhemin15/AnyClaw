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
		scheduler:  NewModelScheduler(),
		client: &http.Client{
			Timeout: 120 * time.Second,
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

	// 检查用户金币（对话消耗用户金币）
	if p.db != nil && userID != "" {
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
	var apiBase, apiKey string
	if len(candidates) > 0 {
		if ep, ok := p.scheduler.Pick(model, candidates); ok {
			apiBase, apiKey = ep.APIBase, ep.APIKey
		}
	}
	if apiBase == "" || apiKey == "" {
		log.Printf("[llm] no key for model %q", model)
		http.Error(w, `{"error":{"message":"no provider configured for model"}}`, http.StatusServiceUnavailable)
		return
	}

	bodyBytes, _ := json.Marshal(req)
	reqURL := strings.TrimSuffix(apiBase, "/") + "/chat/completions"
	proxyReq, err := http.NewRequestWithContext(r.Context(), "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, `{"error":{"message":"internal error"}}`, http.StatusInternalServerError)
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Authorization", "Bearer "+apiKey)
	if u, err := url.Parse(reqURL); err == nil && u.Host != "" {
		host := u.Hostname()
		if p := u.Port(); p != "" && p != "443" && p != "80" {
			host = u.Host
		}
		proxyReq.Host = host
	}

	resp, err := p.client.Do(proxyReq)
	if err != nil {
		log.Printf("[llm] proxy error: url=%s model=%s err=%v", reqURL, model, err)
		http.Error(w, `{"error":{"message":"upstream error"}}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		if strings.ToLower(k) == "content-type" || strings.ToLower(k) == "content-length" {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
	}
	w.WriteHeader(resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Printf("[llm] upstream non-200: url=%s model=%s status=%d body=%s", reqURL, model, resp.StatusCode, string(respBody))
	}
	w.Write(respBody)

	if resp.StatusCode == http.StatusOK {
		promptTokens, completionTokens := parseUsageFromResponse(respBody)
		cost := 0
		if p.db != nil && userID != "" {
			cfg, _ := config.Load(p.configPath)
			tokensPerEnergy := config.GetEnergyConfig(cfg).TokensPerEnergy
			cost = energyFromTokens(promptTokens, completionTokens, tokensPerEnergy)
			uid, _ := strconv.ParseInt(userID, 10, 64)
			if ok, _ := p.db.DeductUserEnergy(uid, cost); !ok {
				log.Printf("[llm] deduct user %d coins %d failed (insufficient balance?)", uid, cost)
			}
		}
		p.logUsage(instanceID, userID, model, apiBase, promptTokens, completionTokens, cost)
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
