package auth

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/mail"
	"github.com/anyclaw/anyclaw-api/internal/ratelimit"
	"github.com/anyclaw/anyclaw-api/internal/request"
)

var (
	sendCodeLimiterIP    = ratelimit.New(5, 15*time.Minute)   // 5 per 15 min per IP
	sendCodeLimiterEmail = ratelimit.New(3, 15*time.Minute)   // 3 per 15 min per email
)

type RegisterRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	Code       string `json:"code"`
	InviteCode string `json:"invite_code"`
}

type SendCodeRequest struct {
	Email string `json:"email"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken string   `json:"access_token"`
	User        *db.User `json:"user"`
}

func clientIP(r *http.Request) string {
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.TrimSpace(strings.Split(x, ",")[0])
	}
	if x := r.Header.Get("X-Real-IP"); x != "" {
		return strings.TrimSpace(x)
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

func genCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(900000))
	return fmt.Sprintf("%06d", n.Int64()+100000)
}

func (a *Auth) smtpConfigured() bool {
	if cfg, err := config.Load(a.configPath); err == nil && cfg.SMTP != nil && cfg.SMTP.Host != "" {
		return true
	}
	_, _, _, _, _, ok := mail.ConfigFromEnv()
	return ok
}

func (a *Auth) smtpParams() *mail.SMTPParams {
	if cfg, err := config.Load(a.configPath); err == nil && cfg.SMTP != nil && cfg.SMTP.Host != "" {
		port := cfg.SMTP.Port
		if port <= 0 {
			port = 587
		}
		return &mail.SMTPParams{
			Host: cfg.SMTP.Host,
			Port: port,
			User: cfg.SMTP.User,
			Pass: cfg.SMTP.Pass,
			From: cfg.SMTP.From,
		}
	}
	host, port, user, pass, from, ok := mail.ConfigFromEnv()
	if !ok {
		return nil
	}
	return &mail.SMTPParams{Host: host, Port: port, User: user, Pass: pass, From: from}
}

func (a *Auth) HandleAuthConfig(w http.ResponseWriter, r *http.Request) {
	ok := a.smtpConfigured()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"email_verification_required": ok})
}

func (a *Auth) HandleSendCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req SendCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		http.Error(w, `{"error":"email required"}`, http.StatusBadRequest)
		return
	}
	if !mail.IsValidEmail(email) {
		http.Error(w, `{"error":"invalid email format"}`, http.StatusBadRequest)
		return
	}
	if !a.smtpConfigured() {
		http.Error(w, `{"error":"邮件服务未配置"}`, http.StatusServiceUnavailable)
		return
	}
	existing, _ := a.db.GetUserByEmail(email)
	if existing != nil {
		http.Error(w, `{"error":"email already registered"}`, http.StatusConflict)
		return
	}
	ip := clientIP(r)
	if !sendCodeLimiterIP.Allow(ip) {
		if sec := sendCodeLimiterIP.RetryAfter(ip); sec > 0 {
			http.Error(w, fmt.Sprintf(`{"error":"请求过于频繁，请 %d 秒后再试"}`, sec), http.StatusTooManyRequests)
			return
		}
	}
	if !sendCodeLimiterEmail.Allow("email:"+email) {
		if sec := sendCodeLimiterEmail.RetryAfter("email:" + email); sec > 0 {
			http.Error(w, fmt.Sprintf(`{"error":"该邮箱发送过于频繁，请 %d 秒后再试"}`, sec), http.StatusTooManyRequests)
			return
		}
	}
	code := genCode()
	expiresAt := time.Now().Add(5 * time.Minute)
	if err := a.db.SaveVerificationCode(email, code, expiresAt); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if err := mail.SendVerificationCode(email, code, a.smtpParams()); err != nil {
		http.Error(w, `{"error":"发送邮件失败"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "验证码已发送"})
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
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email and password required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Password) < 6 {
		http.Error(w, `{"error":"password must be at least 6 characters"}`, http.StatusBadRequest)
		return
	}
	smtpOk := a.smtpConfigured()
	if smtpOk {
		req.Code = strings.TrimSpace(req.Code)
		if req.Code == "" {
			http.Error(w, `{"error":"验证码 required"}`, http.StatusBadRequest)
			return
		}
		ok, err := a.db.VerifyAndConsumeCode(req.Email, req.Code)
		if err != nil {
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, `{"error":"验证码无效或已过期"}`, http.StatusBadRequest)
			return
		}
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
	cfg, _ := config.Load(a.configPath)
	initialEnergy := config.GetEnergyConfig(cfg).NewUserEnergy
	user, err := a.db.CreateUser(req.Email, hash, "user", smtpOk, initialEnergy)
	if err != nil {
		http.Error(w, `{"error":"failed to create user"}`, http.StatusInternalServerError)
		return
	}
	if req.InviteCode != "" {
		if inviterID, err := a.db.UseInvitation(strings.TrimSpace(req.InviteCode), user.ID); err == nil {
			reward := config.GetEnergyConfig(cfg).InviteReward
			_ = a.db.AddUserEnergy(user.ID, reward)
			_ = a.db.AddUserEnergy(inviterID, reward)
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
	if !user.EmailVerified {
		http.Error(w, `{"error":"请先完成邮箱验证"}`, http.StatusForbidden)
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
