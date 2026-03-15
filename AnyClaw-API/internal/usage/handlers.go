package usage

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

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

// SetUsageCorrectionRequest 矫正今日 token 用量
type SetUsageCorrectionRequest struct {
	Provider       string `json:"provider"`        // 渠道名或 api_base，与 usage_log 一致
	TokensDelta    int64  `json:"tokens_delta"`    // 校正量，正数增加、负数减少
	CorrectedTotal *int64 `json:"corrected_total"` // 可选：直接设置为目标值，与 tokens_delta 二选一
}

func (h *Handler) SetUsageCorrection(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req SetUsageCorrectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		http.Error(w, `{"error":"provider required"}`, http.StatusBadRequest)
		return
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	if loc == nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	today := time.Now().In(loc)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)

	delta := req.TokensDelta
	if req.CorrectedTotal != nil {
		rawSum, err := h.db.GetRawUsageForProviderToday(provider)
		if err != nil {
			rawSum = 0
		}
		delta = *req.CorrectedTotal - rawSum
	}

	if err := h.db.SetUsageCorrection(provider, today, delta); err != nil {
		log.Printf("[usage] SetUsageCorrection: %v", err)
		http.Error(w, `{"error":"failed to set correction"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
