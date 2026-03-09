package setup

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/db"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	configPath string
}

func New(configPath string) *Handler {
	return &Handler{configPath: configPath}
}

type StatusResponse struct {
	Configured bool `json:"configured"`
}

type DBRequest struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type AdminRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.Load(h.configPath)
	ok := cfg.DBDSN != "" && cfg.JWTSecret != ""
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(StatusResponse{Configured: ok})
}

func (h *Handler) ConfigureDB(w http.ResponseWriter, r *http.Request) {
	var req DBRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	req.Host = strings.TrimSpace(req.Host)
	req.User = strings.TrimSpace(req.User)
	req.Database = strings.TrimSpace(req.Database)
	if req.Host == "" || req.User == "" || req.Database == "" {
		http.Error(w, `{"error":"host, user, database required"}`, http.StatusBadRequest)
		return
	}
	if req.Port <= 0 {
		req.Port = 3306
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4",
		req.User, req.Password, req.Host, req.Port, req.Database)
	database, err := db.Open(dsn)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, escapeJSON(err.Error())), http.StatusBadRequest)
		return
	}
	defer database.Close()
	// Generate JWT secret if not set
	cfg, _ := config.Load(h.configPath)
	jwtSecret := cfg.JWTSecret
	if jwtSecret == "" {
		b := make([]byte, 32)
		rand.Read(b)
		jwtSecret = base64.URLEncoding.EncodeToString(b)
	}
	sc := &config.SaveConfig{DBDSN: dsn, JWTSecret: jwtSecret}
	if err := config.Save(h.configPath, sc); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"save config: %s"}`, escapeJSON(err.Error())), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) CreateAdmin(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.Load(h.configPath)
	if cfg.DBDSN == "" {
		http.Error(w, `{"error":"database not configured"}`, http.StatusBadRequest)
		return
	}
	database, err := db.Open(cfg.DBDSN)
	if err != nil {
		http.Error(w, `{"error":"database connection failed"}`, http.StatusInternalServerError)
		return
	}
	defer database.Close()

	var req AdminRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || len(req.Password) < 6 {
		http.Error(w, `{"error":"email and password (min 6 chars) required"}`, http.StatusBadRequest)
		return
	}
	hash, err := bcryptHash(req.Password)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	_, err = database.Exec("INSERT INTO users (email, password_hash, role) VALUES (?, ?, 'admin')", req.Email, hash)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			http.Error(w, `{"error":"email already exists"}`, http.StatusConflict)
			return
		}
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, escapeJSON(err.Error())), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func escapeJSON(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`)
}

func bcryptHash(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}
