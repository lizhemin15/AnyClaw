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

func (h *Handler) RedeemCode(w http.ResponseWriter, r *http.Request) {
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
	code := strings.TrimSpace(strings.ToUpper(req.Code))
	if code == "" {
		http.Error(w, `{"error":"code required"}`, http.StatusBadRequest)
		return
	}
	energy, err := h.db.RedeemActivationCode(code, claims.UserID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "energy": energy, "message": "兑换成功"})
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

func (h *Handler) AdminGenerateActivationCodes(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req struct {
		Count int    `json:"count"`
		Energy int   `json:"energy"`
		Memo   string `json:"memo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Count <= 0 || req.Count > 100 {
		http.Error(w, `{"error":"count must be 1-100"}`, http.StatusBadRequest)
		return
	}
	if req.Energy <= 0 {
		http.Error(w, `{"error":"energy must be positive"}`, http.StatusBadRequest)
		return
	}
	codes, err := h.db.CreateActivationCodes(req.Energy, req.Count, claims.UserID, strings.TrimSpace(req.Memo))
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"codes": codes, "count": len(codes), "energy": req.Energy})
}

func (h *Handler) AdminListActivationCodes(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "all"
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}
	list, err := h.db.ListActivationCodes(status, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"failed"}`, http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []*db.ActivationCode{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"items": list})
}

func (h *Handler) AdminVerifyActivationCode(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(strings.ToUpper(req.Code))
	if code == "" {
		http.Error(w, `{"error":"code required"}`, http.StatusBadRequest)
		return
	}
	ac, err := h.db.GetActivationCode(code)
	if err != nil {
		http.Error(w, `{"error":"failed"}`, http.StatusInternalServerError)
		return
	}
	if ac == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"valid": false, "message": "激活码不存在"})
		return
	}
	if ac.UsedBy != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"valid": false, "message": "激活码已使用", "used_by": *ac.UsedBy})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"valid": true, "energy": ac.Energy, "memo": ac.Memo})
}

func (h *Handler) AdminRedeemActivationCode(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req struct {
		Code   string `json:"code"`
		UserID int64  `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(strings.ToUpper(req.Code))
	if code == "" || req.UserID <= 0 {
		http.Error(w, `{"error":"code and user_id required"}`, http.StatusBadRequest)
		return
	}
	energy, err := h.db.RedeemActivationCode(code, req.UserID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "energy": energy, "user_id": req.UserID})
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

func generateCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = codeChars[rand.Intn(len(codeChars))]
	}
	return string(b)
}
