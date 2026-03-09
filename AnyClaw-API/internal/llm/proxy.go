package llm

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/anyclaw/anyclaw-api/internal/config"
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
	cfg     *config.Config
	resolver TokenResolver
	client  *http.Client
	mu      sync.RWMutex
}

func New(cfg *config.Config, resolver TokenResolver) *Proxy {
	return &Proxy{
		cfg:     cfg,
		resolver: resolver,
		client:  &http.Client{},
	}
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
		p.mu.RLock()
		info, ok := p.cfg.InstanceMap.Tokens[token]
		p.mu.RUnlock()
		if ok {
			instanceID = info.InstanceID
			userID = info.UserID
		}
	}
	if instanceID == "" {
		http.Error(w, `{"error":{"message":"invalid token"}}`, http.StatusUnauthorized)
		return
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
	if model == "" {
		http.Error(w, `{"error":{"message":"model required"}}`, http.StatusBadRequest)
		return
	}

	apiBase, apiKey := p.selectProvider(model)
	if apiBase == "" || apiKey == "" {
		log.Printf("[llm] no key for model %q", model)
		http.Error(w, `{"error":{"message":"no provider configured for model"}}`, http.StatusServiceUnavailable)
		return
	}

	url := strings.TrimSuffix(apiBase, "/") + "/chat/completions"
	proxyReq, err := http.NewRequestWithContext(r.Context(), "POST", url, bytes.NewReader(body))
	if err != nil {
		http.Error(w, `{"error":{"message":"internal error"}}`, http.StatusInternalServerError)
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.client.Do(proxyReq)
	if err != nil {
		log.Printf("[llm] proxy error: %v", err)
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
	w.Write(respBody)

	if resp.StatusCode == http.StatusOK {
		p.logUsage(instanceID, userID, model, apiBase, respBody)
	}
}

func (p *Proxy) selectProvider(model string) (apiBase, apiKey string) {
	model = strings.ToLower(model)
	if strings.Contains(model, "claude") || strings.HasPrefix(model, "anthropic/") {
		if p.cfg.KeyPool.Anthropic.APIKey != "" {
			return p.cfg.KeyPool.Anthropic.APIBase, p.cfg.KeyPool.Anthropic.APIKey
		}
	}
	if strings.Contains(model, "gpt") || strings.HasPrefix(model, "openai/") || !strings.Contains(model, "/") {
		if p.cfg.KeyPool.OpenAI.APIKey != "" {
			return p.cfg.KeyPool.OpenAI.APIBase, p.cfg.KeyPool.OpenAI.APIKey
		}
	}
	if p.cfg.KeyPool.OpenRouter.APIKey != "" {
		return p.cfg.KeyPool.OpenRouter.APIBase, p.cfg.KeyPool.OpenRouter.APIKey
	}
	if p.cfg.KeyPool.OpenAI.APIKey != "" {
		return p.cfg.KeyPool.OpenAI.APIBase, p.cfg.KeyPool.OpenAI.APIKey
	}
	return "", ""
}

func (p *Proxy) logUsage(instanceID, userID, model, provider string, respBody []byte) {
	var usage struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(respBody, &usage)
	rec := UsageRecord{
		InstanceID:       instanceID,
		UserID:           userID,
		Model:            model,
		Provider:         provider,
		PromptTokens:     usage.Usage.PromptTokens,
		CompletionTokens: usage.Usage.CompletionTokens,
	}
	// TODO: persist to DB; for now just log
	log.Printf("[usage] %+v", rec)
}

func extractBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}
