package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/request"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type Auth struct {
	db         *db.DB
	secret     []byte
	configPath string
}

func New(database *db.DB, secret, configPath string) *Auth {
	if secret == "" {
		b := make([]byte, 32)
		rand.Read(b)
		secret = base64.URLEncoding.EncodeToString(b)
	}
	return &Auth{db: database, secret: []byte(secret), configPath: configPath}
}

func (a *Auth) HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}

func (a *Auth) CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (a *Auth) CreateToken(user *db.User) (string, error) {
	claims := Claims{
		UserID: user.ID,
		Role:   user.Role,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString(a.secret)
}

func (a *Auth) ParseToken(tokenString string) (*Claims, error) {
	t, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return a.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := t.Claims.(*Claims); ok && t.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid token")
}

func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearer(r)
		if token == "" {
			writeUnauth(w, r, "missing authorization")
			return
		}
		claims, err := a.ParseToken(token)
		if err != nil {
			writeUnauth(w, r, "invalid token")
			return
		}
		rc := &request.Claims{UserID: claims.UserID, Role: claims.Role, Email: claims.Email}
		ctx := request.WithClaims(r.Context(), rc)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// writeUnauth 对浏览器导航请求重定向到登录页，避免展示 raw JSON
func writeUnauth(w http.ResponseWriter, r *http.Request, errMsg string) {
	if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html") {
		path := r.URL.Path
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, "/login?return_to="+url.QueryEscape(path), http.StatusFound)
		return
	}
	http.Error(w, `{"error":"`+errMsg+`"}`, http.StatusUnauthorized)
}

func extractBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	// WebSocket: browsers cannot set headers, so allow token in query
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return ""
}
