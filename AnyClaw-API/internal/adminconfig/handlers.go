package adminconfig

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/request"
)

type Handler struct {
	configPath string
}

func New(configPath string) *Handler {
	return &Handler{configPath: configPath}
}

func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	cfg, err := config.Load(h.configPath)
	if err != nil {
		http.Error(w, `{"error":"failed to load config"}`, http.StatusInternalServerError)
		return
	}
	channels := cfg.Channels
	if channels == nil {
		channels = []config.Channel{}
	}
	// Mask API keys for response
	out := make([]map[string]any, len(channels))
	for i, ch := range channels {
		out[i] = map[string]any{
			"id":      ch.ID,
			"name":    ch.Name,
			"api_key": config.MaskAPIKey(ch.APIKey),
			"api_base": ch.APIBase,
			"enabled": ch.Enabled,
			"models":  ch.Models,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"channels": out})
}

func (h *Handler) PutConfig(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req struct {
		Channels []config.Channel `json:"channels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	cfg, err := config.Load(h.configPath)
	if err != nil {
		http.Error(w, `{"error":"failed to load config"}`, http.StatusInternalServerError)
		return
	}
	// Merge: preserve existing api_key if client sent masked value
	channels := req.Channels
	if channels == nil {
		channels = []config.Channel{}
	}
	existing := make(map[string]string)
	for _, ch := range cfg.Channels {
		existing[ch.ID] = ch.APIKey
	}
	for i := range channels {
		if k, ok := existing[channels[i].ID]; ok && (channels[i].APIKey == "" || strings.HasPrefix(channels[i].APIKey, "****")) {
			channels[i].APIKey = k
		}
	}
	if err := config.SaveAdminConfig(h.configPath, channels); err != nil {
		http.Error(w, `{"error":"failed to save config"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

// TestChannelRequest 测试渠道/模型连通性
type TestChannelRequest struct {
	ChannelID string `json:"channel_id"` // 从已保存配置查找
	Model     string `json:"model"`      // 模型名
	// 或直接传凭证（测试未保存的配置）
	APIBase string `json:"api_base"`
	APIKey  string `json:"api_key"`
}

func (h *Handler) TestChannel(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req TestChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	apiBase, apiKey, model := "", "", ""
	if req.APIBase != "" && req.APIKey != "" {
		apiBase = strings.TrimSuffix(strings.TrimSpace(req.APIBase), "/")
		if apiBase == "" {
			apiBase = "https://api.openai.com/v1"
		}
		apiKey = strings.TrimSpace(req.APIKey)
		model = strings.TrimSpace(req.Model)
		if model == "" {
			model = "gpt-4o"
		}
	} else if req.ChannelID != "" && req.Model != "" {
		cfg, err := config.Load(h.configPath)
		if err != nil {
			http.Error(w, `{"error":"failed to load config"}`, http.StatusInternalServerError)
			return
		}
		model = strings.TrimSpace(req.Model)
		for _, ch := range cfg.Channels {
			if ch.ID != req.ChannelID {
				continue
			}
			apiKey = ch.APIKey
			apiBase = ch.APIBase
			if apiBase == "" {
				apiBase = "https://api.openai.com/v1"
			}
			apiBase = strings.TrimSuffix(apiBase, "/")
			break
		}
		if apiKey == "" {
			http.Error(w, `{"error":"channel not found or no api key"}`, http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, `{"error":"provide channel_id+model or api_base+api_key+model"}`, http.StatusBadRequest)
		return
	}

	url := apiBase + "/chat/completions"
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
		"max_tokens": 5,
	}
	bodyBytes, _ := json.Marshal(body)
	proxyReq, err := http.NewRequestWithContext(r.Context(), "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("[admin] config test failed: url=%s model=%s err=%v", url, model, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"ok":      false,
			"message": "连接失败: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("[admin] config test non-200: url=%s model=%s status=%d body=%s", url, model, resp.StatusCode, string(respBody))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"ok":      false,
			"message": "上游返回 " + http.StatusText(resp.StatusCode) + ": " + string(respBody),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"message": "连接正常",
	})
}
