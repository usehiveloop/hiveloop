package middleware

import (
	"context"
	"crypto/rsa"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/model"
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

// ResolveOrgFromHeader returns middleware that reads the X-Org-ID header,
// validates the user is a member of that org, and sets the org on context.
func ResolveOrgFromHeader(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			orgIDStr := r.Header.Get("X-Org-ID")
			if orgIDStr == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing X-Org-ID header"})
				return
			}

			orgID, err := uuid.Parse(orgIDStr)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid X-Org-ID header"})
				return
			}

			claims, ok := AuthClaimsFromContext(r.Context())
			if !ok || claims.UserID == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing auth context"})
				return
			}

			var membership model.OrgMembership
			if err := db.Preload("Org").Where("user_id = ? AND org_id = ?", claims.UserID, orgID).First(&membership).Error; err != nil {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of the requested organization"})
				return
			}

			if !membership.Org.Active {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "organization is inactive"})
				return
			}

			next.ServeHTTP(w, WithOrg(r, &membership.Org))
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
				slog.Warn("resolve user: missing auth claims or user_id", "has_claims", ok, "path", r.URL.Path)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
				return
			}

			var user model.User
			if err := db.Where("id = ?", claims.UserID).First(&user).Error; err != nil {
				slog.Warn("resolve user: user not found in database", "user_id", claims.UserID, "error", err)
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "user not found"})
				return
			}

			next.ServeHTTP(w, WithUser(r, &user))
		})
	}
}

// RequireOrgAdmin enforces that the authenticated user's role in the resolved
// org context is "owner" or "admin". Must be used AFTER a ResolveOrg* middleware.
func RequireOrgAdmin(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := AuthClaimsFromContext(r.Context())
			if !ok || claims == nil || claims.UserID == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			org, ok := OrgFromContext(r.Context())
			if !ok || org == nil {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing organization context"})
				return
			}
			var m model.OrgMembership
			if err := db.Where("user_id = ? AND org_id = ?", claims.UserID, org.ID).First(&m).Error; err != nil {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member"})
				return
			}
			if m.Role != "owner" && m.Role != "admin" {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
				return
			}
			next.ServeHTTP(w, r)
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
				slog.Warn("platform admin check: no user in context", "path", r.URL.Path)
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}
			if !emailSet[user.Email] {
				slog.Warn("platform admin check: email not in allowlist", "email", user.Email, "path", r.URL.Path)
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "platform admin access required"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
