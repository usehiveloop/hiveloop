package middleware

import (
	"context"
	"net/http"

	"github.com/useportal/proxy-bridge/internal/model"
)

type contextKey int

const (
	orgKey contextKey = iota
	claimsKey
)

// OrgFromContext retrieves the authenticated Org from the request context.
func OrgFromContext(ctx context.Context) (*model.Org, bool) {
	org, ok := ctx.Value(orgKey).(*model.Org)
	return org, ok
}

// WithOrg sets the Org on the request context.
func WithOrg(r *http.Request, org *model.Org) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), orgKey, org))
}

// TokenClaims holds the extracted claims from a validated sandbox token.
type TokenClaims struct {
	OrgID        string
	CredentialID string
	JTI          string
}

// ClaimsFromContext retrieves the token claims from the request context.
func ClaimsFromContext(ctx context.Context) (*TokenClaims, bool) {
	claims, ok := ctx.Value(claimsKey).(*TokenClaims)
	return claims, ok
}

// WithClaims sets the token claims on the request context.
func WithClaims(r *http.Request, claims *TokenClaims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), claimsKey, claims))
}
