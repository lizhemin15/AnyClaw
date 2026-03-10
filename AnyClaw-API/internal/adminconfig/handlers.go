package adminconfig

import (
	"encoding/json"
	"net/http"
	"strings"

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
	modelList := cfg.ModelList
	if modelList == nil {
		modelList = []config.ModelEntry{}
	}
	resp := map[string]any{
		"model_list": modelList,
		"key_pool": map[string]any{
			"openai": map[string]any{
				"api_key":  config.MaskAPIKey(cfg.KeyPool.OpenAI.APIKey),
				"api_base": cfg.KeyPool.OpenAI.APIBase,
			},
			"anthropic": map[string]any{
				"api_key":  config.MaskAPIKey(cfg.KeyPool.Anthropic.APIKey),
				"api_base": cfg.KeyPool.Anthropic.APIBase,
			},
			"openrouter": map[string]any{
				"api_key":  config.MaskAPIKey(cfg.KeyPool.OpenRouter.APIKey),
				"api_base": cfg.KeyPool.OpenRouter.APIBase,
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) PutConfig(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req struct {
		ModelList []config.ModelEntry `json:"model_list"`
		KeyPool   config.KeyPool      `json:"key_pool"`
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
	// Merge: only overwrite non-empty fields from request
	mergeKeyEntry(&cfg.KeyPool.OpenAI, &req.KeyPool.OpenAI)
	mergeKeyEntry(&cfg.KeyPool.Anthropic, &req.KeyPool.Anthropic)
	mergeKeyEntry(&cfg.KeyPool.OpenRouter, &req.KeyPool.OpenRouter)
	modelList := req.ModelList
	if modelList == nil {
		modelList = []config.ModelEntry{}
	}
	if err := config.SaveAdminConfig(h.configPath, cfg.KeyPool, modelList); err != nil {
		http.Error(w, `{"error":"failed to save config"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func mergeKeyEntry(dst, src *config.KeyEntry) {
	// Skip if api_key looks like masked value (user didn't change it)
	if src.APIKey != "" && !strings.HasPrefix(src.APIKey, "****") {
		dst.APIKey = src.APIKey
	}
	if src.APIBase != "" {
		dst.APIBase = src.APIBase
	}
}
