package energy

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"strconv"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/request"
	"github.com/go-chi/chi/v5"
)

const codeChars = "abcdefghijklmnopqrstuvwxyz0123456789"

type Handler struct {
	db         *db.DB
	configPath string
}

func New(db *db.DB, configPath string) *Handler {
	return &Handler{db: db, configPath: configPath}
}

func (h *Handler) Recharge(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req struct {
		UserID int `json:"user_id"`
		Amount int `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Amount <= 0 {
		http.Error(w, `{"error":"amount must be positive"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.AddUserEnergy(int64(req.UserID), req.Amount); err != nil {
		http.Error(w, `{"error":"failed to recharge"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (h *Handler) InviteCode(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	code := generateCode(8)
	if err := h.db.CreateInvitation(claims.UserID, code); err != nil {
		http.Error(w, `{"error":"failed to create code"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"code": code})
}

func (h *Handler) UseInviteCode(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		http.Error(w, `{"error":"code required"}`, http.StatusBadRequest)
		return
	}
	inviterID, err := h.db.UseInvitation(code, claims.UserID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	cfg, _ := config.Load(h.configPath)
	reward := config.GetEnergyConfig(cfg).InviteReward
	_ = h.db.AddUserEnergy(claims.UserID, reward)
	_ = h.db.AddUserEnergy(inviterID, reward)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "reward": reward})
}

func (h *Handler) RunDaily(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	// 已改为全部扣用户金币，不再对实例做每日扣费，也不再按零活力删除
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "deleted": 0})
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	list, err := h.db.ListUsers()
	if err != nil {
		http.Error(w, `{"error":"failed"}`, http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []*db.User{}
	}
	type userWithInstances struct {
		*db.User
		InstanceCount int `json:"instance_count"`
	}
	out := make([]userWithInstances, len(list))
	for i, u := range list {
		count, _ := h.db.CountInstancesByUserID(u.ID)
		out[i] = userWithInstances{User: u, InstanceCount: count}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (h *Handler) AdminRechargeUser(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	userID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var req struct {
		Amount int `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Amount <= 0 {
		http.Error(w, `{"error":"invalid amount"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.AddUserEnergy(userID, req.Amount); err != nil {
		http.Error(w, `{"error":"failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func generateCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = codeChars[rand.Intn(len(codeChars))]
	}
	return string(b)
}
