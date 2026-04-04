package middleware

import (
	"context"
	"crypto/rsa"
	"log/slog"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/auth"
	"github.com/llmvault/llmvault/internal/model"
)

type authClaimsKey struct{}

// AuthClaimsFromContext retrieves embedded auth JWT claims from the request context.
func AuthClaimsFromContext(ctx context.Context) (*auth.AuthClaims, bool) {
	claims, ok := ctx.Value(authClaimsKey{}).(*auth.AuthClaims)
	return claims, ok
}

// WithAuthClaims sets the embedded auth claims on the request context.
func WithAuthClaims(r *http.Request, claims *auth.AuthClaims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), authClaimsKey{}, claims))
}

// RequireAuth returns middleware that validates an RS256 Bearer JWT and sets
// AuthClaims on the context.
func RequireAuth(pubKey *rsa.PublicKey, issuer, audience string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractBearerToken(r)
			if tokenStr == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization token"})
				return
			}

			claims, err := auth.ValidateAccessToken(pubKey, issuer, audience, tokenStr)
			if err != nil {
				slog.Warn("token validation failed", "error", err)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
				return
			}

			next.ServeHTTP(w, WithAuthClaims(r, claims))
		})
	}
}

// ResolveOrgFromClaims returns middleware that reads org_id from AuthClaims,
// loads the Org by primary key, and sets it on the context.
func ResolveOrgFromClaims(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := AuthClaimsFromContext(r.Context())
			if !ok || claims.OrgID == "" {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing organization context"})
				return
			}

			var org model.Org
			if err := db.Where("id = ?", claims.OrgID).First(&org).Error; err != nil {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "organization not found"})
				return
			}

			if !org.Active {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "organization is inactive"})
				return
			}

			next.ServeHTTP(w, WithOrg(r, &org))
		})
	}
}

// ResolveUser returns middleware that reads user_id from AuthClaims,
// loads the User by primary key, and sets it on the context.
func ResolveUser(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := AuthClaimsFromContext(r.Context())
			if !ok || claims.UserID == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
				return
			}

			var user model.User
			if err := db.Where("id = ?", claims.UserID).First(&user).Error; err != nil {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "user not found"})
				return
			}

			next.ServeHTTP(w, WithUser(r, &user))
		})
	}
}

// RequirePlatformAdmin returns middleware that checks if the authenticated user's
// email is in the platform admin allowlist.
func RequirePlatformAdmin(adminEmails []string) func(http.Handler) http.Handler {
	emailSet := make(map[string]bool, len(adminEmails))
	for _, e := range adminEmails {
		trimmed := strings.TrimSpace(e)
		if trimmed != "" {
			emailSet[trimmed] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := UserFromContext(r.Context())
			if !ok {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}
			if !emailSet[user.Email] {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "platform admin access required"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
