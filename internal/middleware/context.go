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
	connectSessionKey
	identityKey
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

// ConnectSessionFromContext retrieves the connect session from the request context.
func ConnectSessionFromContext(ctx context.Context) (*model.ConnectSession, bool) {
	sess, ok := ctx.Value(connectSessionKey).(*model.ConnectSession)
	return sess, ok
}

// WithConnectSession sets the connect session on the request context.
func WithConnectSession(r *http.Request, sess *model.ConnectSession) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), connectSessionKey, sess))
}

// IdentityFromContext retrieves the identity from the request context.
func IdentityFromContext(ctx context.Context) (*model.Identity, bool) {
	ident, ok := ctx.Value(identityKey).(*model.Identity)
	return ident, ok
}

// WithIdentity sets the identity on the request context.
func WithIdentity(r *http.Request, ident *model.Identity) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identityKey, ident))
}
