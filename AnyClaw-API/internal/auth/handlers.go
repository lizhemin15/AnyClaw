package auth

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
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
	params := a.smtpParamsFromConfig()
	if params != nil && params.Pass != "" && !strings.HasPrefix(params.Pass, "****") {
		return params
	}
	// 配置中密码为空或脱敏时，直接从 DB 读取（注册/发验证码与测试接口同源）
	if p := a.smtpParamsFromDB(); p != nil {
		return p
	}
	if params != nil {
		return params
	}
	host, port, user, pass, from, ok := mail.ConfigFromEnv()
	if !ok {
		return nil
	}
	return &mail.SMTPParams{Host: host, Port: port, User: user, Pass: pass, From: from}
}

func (a *Auth) smtpParamsFromConfig() *mail.SMTPParams {
	cfg, err := config.Load(a.configPath)
	if err != nil || cfg.SMTP == nil || cfg.SMTP.Host == "" {
		return nil
	}
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

func (a *Auth) smtpParamsFromDB() *mail.SMTPParams {
	if a.db == nil {
		return nil
	}
	b, err := a.db.GetAdminConfigJSON()
	if err != nil || len(b) == 0 {
		return nil
	}
	var raw struct {
		SMTP *struct {
			Host string `json:"host"`
			Port int    `json:"port"`
			User string `json:"user"`
			Pass string `json:"pass"`
			From string `json:"from"`
		} `json:"smtp"`
	}
	if json.Unmarshal(b, &raw) != nil || raw.SMTP == nil || raw.SMTP.Host == "" {
		return nil
	}
	s := raw.SMTP
	if s.Pass == "" || strings.HasPrefix(s.Pass, "****") {
		return nil
	}
	port := s.Port
	if port <= 0 {
		port = 587
	}
	from := s.From
	if from == "" {
		from = s.User
	}
	return &mail.SMTPParams{Host: s.Host, Port: port, User: s.User, Pass: s.Pass, From: from}
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
	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	if err := a.db.SaveVerificationCode(email, code, expiresAt); err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	params := a.smtpParams()
	if params == nil || params.Pass == "" || strings.HasPrefix(params.Pass, "****") {
		log.Printf("[auth] send verification code: SMTP password not available (missing or masked)")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "发送邮件失败（SMTP 密码未正确配置，请在管理后台重新填写邮箱密码并保存）"})
		return
	}
	if err := mail.SendVerificationCode(email, code, params); err != nil {
		log.Printf("[auth] send verification code to %s failed: %v", email, err)
		msg := "发送邮件失败"
		errLower := strings.ToLower(err.Error())
		if strings.Contains(errLower, "535") || strings.Contains(errLower, "authentication") {
			msg = "发送邮件失败（SMTP 认证错误，163/QQ 等须用授权码而非登录密码）"
		} else if strings.Contains(errLower, "550") || strings.Contains(errLower, "invalid user") {
			msg = "发送邮件失败（550 Invalid User：163/QQ 等要求发件人必须与 SMTP 登录账号一致，请在管理后台将「发件人」设为与「用户名」相同的完整邮箱）"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": msg})
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
			log.Printf("[auth] verify code for %s failed: %v", req.Email, err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		if !ok {
			log.Printf("[auth] verify code failed for %s: code invalid or expired", req.Email)
			http.Error(w, `{"error":"验证码无效或已过期，请重新获取"}`, http.StatusBadRequest)
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
		log.Printf("[auth] create user %s failed: %v", req.Email, err)
		msg := "注册失败，请稍后重试"
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "1062") {
			msg = "该邮箱已注册，请直接登录"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": msg})
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
	cfg, _ := config.Load(a.configPath)
	bonus := config.GetEnergyConfig(cfg).DailyLoginBonus
	if granted, _ := a.db.GrantDailyLoginBonus(user.ID, bonus); granted {
		user, _ = a.db.GetUserByID(user.ID)
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
