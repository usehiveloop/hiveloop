package middleware

import (
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/model"
)

// ConnectSessionAuth returns middleware that authenticates requests using
// connect session tokens (csess_...). It validates the token, checks expiry,
// validates the Origin header, marks activation, and sets session/org/identity
// on the request context.
func ConnectSessionAuth(db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for CORS preflight
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization header"})
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid authorization format"})
				return
			}

			rawToken := parts[1]
			if !strings.HasPrefix(rawToken, "csess_") {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid session token format"})
				return
			}

			// Look up session in DB with org preloaded
			var sess model.ConnectSession
			if err := db.Preload("Org").Where("session_token = ?", rawToken).First(&sess).Error; err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid session token"})
				return
			}

			// Check expiration
			if time.Now().After(sess.ExpiresAt) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session expired"})
				return
			}

			// Check org is active
			if !sess.Org.Active {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "organization is inactive"})
				return
			}

			// Validate Origin header against session's allowed_origins
			origin := r.Header.Get("Origin")
			if origin != "" && len(sess.AllowedOrigins) > 0 {
				if !containsOrigin(sess.AllowedOrigins, origin) {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin not allowed"})
					return
				}
			}

			// Mark activated on first use
			if sess.ActivatedAt == nil {
				now := time.Now()
				db.Model(&sess).Update("activated_at", &now)
				sess.ActivatedAt = &now
			}

			// Load identity if linked
			r = WithConnectSession(r, &sess)
			r = WithOrg(r, &sess.Org)

			if sess.IdentityID != nil {
				var ident model.Identity
				if err := db.Where("id = ?", *sess.IdentityID).First(&ident).Error; err == nil {
					r = WithIdentity(r, &ident)
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func containsOrigin(allowed []string, origin string) bool {
	for _, a := range allowed {
		if a == origin || a == "*" {
			return true
		}
	}
	return false
}
