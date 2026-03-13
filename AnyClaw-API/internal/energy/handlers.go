package energy

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/request"
	"github.com/go-chi/chi/v5"
)

type PasswordHasher interface {
	HashPassword(password string) (string, error)
}

type Handler struct {
	db         *db.DB
	configPath string
	auth       PasswordHasher
}

func New(db *db.DB, configPath string, auth PasswordHasher) *Handler {
	return &Handler{db: db, configPath: configPath, auth: auth}
}

// GetPublicConfig 公开的金币配置（领养价格等），无需登录
func (h *Handler) GetPublicConfig(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.Load(h.configPath)
	ec := config.GetEnergyConfig(cfg)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"adopt_cost":                ec.AdoptCost,
		"monthly_subscription_cost": ec.MonthlySubscriptionCost,
		"tokens_per_energy":         ec.TokensPerEnergy,
		"min_energy_for_task":       ec.MinEnergyForTask,
	})
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

var defaultRechargePlans = []config.PaymentPlan{
	{ID: "plan-1", Name: "入门", Benefits: "100 金币", Energy: 100, PriceCny: 100, Sort: 0},
	{ID: "plan-2", Name: "进阶", Benefits: "500 金币", Energy: 500, PriceCny: 450, Sort: 1},
	{ID: "plan-3", Name: "尊享", Benefits: "2000 金币", Energy: 2000, PriceCny: 1600, Sort: 2},
}

// GetRechargePlans 获取充值档位（三档，管理员可配置）
func (h *Handler) GetRechargePlans(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	cfg, err := config.Load(h.configPath)
	if err != nil || cfg.Payment == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(defaultRechargePlans)
		return
	}
	plans := cfg.Payment.Plans
	if plans == nil {
		plans = []config.PaymentPlan{}
	}
	out := make([]config.PaymentPlan, 3)
	for i := 0; i < 3; i++ {
		if i < len(plans) {
			out[i] = plans[i]
			out[i].ID = defaultRechargePlans[i].ID
			out[i].Sort = i
			if out[i].Benefits == "" && out[i].Energy > 0 {
				out[i].Benefits = strconv.Itoa(out[i].Energy) + " 金币"
			}
		} else {
			out[i] = defaultRechargePlans[i]
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
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

func (h *Handler) AdminCreateUser(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	if h.auth == nil {
		http.Error(w, `{"error":"auth not configured"}`, http.StatusInternalServerError)
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
		Energy   int    `json:"energy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" || len(req.Password) < 6 {
		http.Error(w, `{"error":"email required, password at least 6 chars"}`, http.StatusBadRequest)
		return
	}
	if req.Role != "admin" && req.Role != "user" {
		req.Role = "user"
	}
	if req.Energy < 0 {
		req.Energy = 0
	}
	hash, err := h.auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, `{"error":"hash failed"}`, http.StatusInternalServerError)
		return
	}
	u, err := h.db.CreateUser(email, hash, req.Role, true, req.Energy)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "UNIQUE") {
			http.Error(w, `{"error":"邮箱已存在"}`, http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "user": map[string]any{"id": u.ID, "email": u.Email, "role": u.Role, "energy": u.Energy}})
}

func (h *Handler) AdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	userID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if userID <= 0 {
		http.Error(w, `{"error":"invalid user id"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		Role   *string `json:"role"`
		Energy *int    `json:"energy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Role != nil {
		role := strings.TrimSpace(*req.Role)
		if role != "user" && role != "admin" {
			role = "user"
		}
		if err := h.db.UpdateUserRole(userID, role); err != nil {
			http.Error(w, `{"error":"update role failed"}`, http.StatusInternalServerError)
			return
		}
	}
	if req.Energy != nil {
		e := *req.Energy
		if e < 0 {
			e = 0
		}
		if err := h.db.SetUserEnergy(userID, e); err != nil {
			http.Error(w, `{"error":"update energy failed"}`, http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
