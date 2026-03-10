package adminstats

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/request"
)

type Handler struct {
	db *db.DB
}

func New(database *db.DB) *Handler {
	return &Handler{db: database}
}

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	days := 7
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 90 {
			days = n
		}
	}
	since := time.Now().AddDate(0, 0, -days)
	stats, err := h.db.GetUsageStats(since)
	if err != nil {
		http.Error(w, `{"error":"failed to get stats"}`, http.StatusInternalServerError)
		return
	}
	// Enrich ByUser with email from users table
	for i := range stats.ByUser {
		if id, err := strconv.ParseInt(stats.ByUser[i].UserID, 10, 64); err == nil {
			if u, _ := h.db.GetUserByID(id); u != nil {
				stats.ByUser[i].Email = u.Email
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
