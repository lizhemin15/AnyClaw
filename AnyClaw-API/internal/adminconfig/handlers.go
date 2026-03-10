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
