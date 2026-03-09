package request

import "context"

// Claims holds user info from JWT, stored in request context.
type Claims struct {
	UserID int64
	Role   string
	Email  string
}

type contextKey string

const claimsKey contextKey = "claims"

func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

func FromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsKey).(*Claims)
	return c
}
