package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/energy"
	"github.com/anyclaw/anyclaw-api/internal/request"
)

type RegisterRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	InviteCode string `json:"invite_code"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken string     `json:"access_token"`
	User        *db.User   `json:"user"`
}

func (a *Auth) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email and password required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Password) < 6 {
		http.Error(w, `{"error":"password must be at least 6 characters"}`, http.StatusBadRequest)
		return
	}
	existing, _ := a.db.GetUserByEmail(req.Email)
	if existing != nil {
		http.Error(w, `{"error":"email already registered"}`, http.StatusConflict)
		return
	}
	hash, err := a.HashPassword(req.Password)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	user, err := a.db.CreateUser(req.Email, hash, "user")
	if err != nil {
		http.Error(w, `{"error":"failed to create user"}`, http.StatusInternalServerError)
		return
	}
	if req.InviteCode != "" {
		if inviterID, err := a.db.UseInvitation(strings.TrimSpace(req.InviteCode), user.ID); err == nil {
			_ = a.db.AddUserEnergy(user.ID, energy.InviteReward)
			_ = a.db.AddUserEnergy(inviterID, energy.InviteReward)
		}
	}
	token, err := a.CreateToken(user)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{AccessToken: token, User: user})
}

func (a *Auth) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email and password required"}`, http.StatusBadRequest)
		return
	}
	user, err := a.db.GetUserByEmail(req.Email)
	if err != nil || user == nil {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}
	if !a.CheckPassword(user.PasswordHash, req.Password) {
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}
	token, err := a.CreateToken(user)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{AccessToken: token, User: user})
}

func (a *Auth) HandleMe(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	user, err := a.db.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}
