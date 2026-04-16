package middleware

import (
	"context"
	"net/http"

	"github.com/ziraloop/ziraloop/internal/model"
)

type contextKey int

const (
	orgKey contextKey = iota
	claimsKey
	apiKeyClaimsKey
	userKey
	adminAuditChangesKey
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

// APIKeyClaims holds extracted claims from a validated API key.
type APIKeyClaims struct {
	KeyID  string
	OrgID  string
	Scopes []string
}

// APIKeyClaimsFromContext retrieves API key claims from the request context.
func APIKeyClaimsFromContext(ctx context.Context) (*APIKeyClaims, bool) {
	claims, ok := ctx.Value(apiKeyClaimsKey).(*APIKeyClaims)
	return claims, ok
}

// WithAPIKeyClaims sets the API key claims on the request context.
func WithAPIKeyClaims(r *http.Request, claims *APIKeyClaims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), apiKeyClaimsKey, claims))
}

// UserFromContext retrieves the authenticated User from the request context.
func UserFromContext(ctx context.Context) (*model.User, bool) {
	user, ok := ctx.Value(userKey).(*model.User)
	return user, ok
}

// WithUser sets the User on the request context.
func WithUser(r *http.Request, user *model.User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userKey, user))
}

// AdminAuditChanges is a map of field→{old,new} diffs set by admin update
// handlers so the audit middleware logs only what actually changed.
type AdminAuditChanges map[string]any

// AdminAuditBucket is a shared pointer that the middleware allocates and places
// on the context before the handler runs. The handler stores its changes map
// in it, and the middleware reads it after the handler returns.
type AdminAuditBucket struct {
	Changes AdminAuditChanges
}

func AdminAuditBucketFromContext(ctx context.Context) *AdminAuditBucket {
	bucket, _ := ctx.Value(adminAuditChangesKey).(*AdminAuditBucket)
	return bucket
}

func WithAdminAuditBucket(r *http.Request, bucket *AdminAuditBucket) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), adminAuditChangesKey, bucket))
}

func SetAdminAuditChanges(r *http.Request, changes AdminAuditChanges) {
	if bucket := AdminAuditBucketFromContext(r.Context()); bucket != nil {
		bucket.Changes = changes
	}
}
