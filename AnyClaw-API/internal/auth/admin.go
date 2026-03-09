package auth

import (
	"net/http"

	"github.com/anyclaw/anyclaw-api/internal/request"
)

// AdminMiddleware requires the user to have role "admin".
func (a *Auth) AdminMiddleware(next http.Handler) http.Handler {
	return a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := request.FromContext(r.Context())
		if claims == nil || claims.Role != "admin" {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}
