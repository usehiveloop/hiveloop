package middleware

import (
	"context"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
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
	// IsSystem is true when the credential behind this token is
	// platform-owned. The proxy uses it to gate on credit balance and
	// meter token spend. Mirrored from token.Claims.IsSystem.
	IsSystem bool
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

// DistinctID resolves a stable identifier for observability (error tracking,
// analytics). The resolution order is:
//
//  1. Authenticated user (UserFromContext).
//  2. Auth JWT claims (AuthClaimsFromContext).
//  3. Resolved org (OrgFromContext) — prefixed "org:".
//  4. Sandbox proxy token claims (ClaimsFromContext) — prefixed "org:".
//  5. API key claims (APIKeyClaimsFromContext) — prefixed "org:".
//  6. Chi request ID — prefixed "req:".
//  7. Empty string.
//
// Returns "" when nothing is available so the caller can decide whether to
// use "system" or skip the observation entirely.
func DistinctID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if user, ok := UserFromContext(ctx); ok && user != nil && user.ID != uuid.Nil {
		return user.ID.String()
	}

	if claims, ok := AuthClaimsFromContext(ctx); ok && claims != nil && claims.UserID != "" {
		return claims.UserID
	}

	if org, ok := OrgFromContext(ctx); ok && org != nil && org.ID != uuid.Nil {
		return "org:" + org.ID.String()
	}

	if claims, ok := ClaimsFromContext(ctx); ok && claims != nil && claims.OrgID != "" {
		return "org:" + claims.OrgID
	}

	if claims, ok := APIKeyClaimsFromContext(ctx); ok && claims != nil && claims.OrgID != "" {
		return "org:" + claims.OrgID
	}

	if requestID := chimw.GetReqID(ctx); requestID != "" {
		return "req:" + requestID
	}

	return ""
}
