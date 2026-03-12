package usage

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/request"
)

type Handler struct {
	db *db.DB
}

func New(db *db.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) ListAdminUsage(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	limit := 100
	offset := 0
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}
	list, err := h.db.ListAdminUsage(limit, offset)
	if err != nil {
		log.Printf("[usage] ListAdminUsage: %v", err)
		list = []*db.UsageLogEntryAdmin{}
	}
	if list == nil {
		list = []*db.UsageLogEntryAdmin{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"items": list})
}

func (h *Handler) ListMyUsage(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	limit := 50
	offset := 0
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			offset = n
		}
	}
	list, err := h.db.ListUserUsage(claims.UserID, limit, offset)
	if err != nil {
		log.Printf("[usage] ListUserUsage: %v", err)
		// 表不存在或查询失败时返回空列表，避免页面报错；用户可点击「检查修复数据库」修复
		list = []*db.UsageLogEntry{}
	}
	if list == nil {
		list = []*db.UsageLogEntry{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"items": list})
}
