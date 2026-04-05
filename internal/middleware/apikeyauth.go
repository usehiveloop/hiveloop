package middleware

import (
	"crypto/rsa"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/cache"
	"github.com/ziraloop/ziraloop/internal/enqueue"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/tasks"
)

// APIKeyAuth returns middleware that authenticates requests using self-issued API keys (zira_sk_*).
// It checks the in-memory cache first, then falls back to the database.
func APIKeyAuth(db *gorm.DB, keyCache *cache.APIKeyCache, enqueuer enqueue.TaskEnqueuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawKey := extractBearerToken(r)
			if rawKey == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization header"})
				return
			}

			if !strings.HasPrefix(rawKey, "zira_sk_") {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid api key format"})
				return
			}

			keyHash := model.HashAPIKey(rawKey)

			// L1: Check in-memory cache
			if cached, ok := keyCache.Get(keyHash); ok {
				var org model.Org
				if err := db.Where("id = ?", cached.OrgID).First(&org).Error; err != nil {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "organization not found"})
					return
				}
				if !org.Active {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "organization is inactive"})
					return
				}

				claims := &APIKeyClaims{
					KeyID:  cached.ID.String(),
					OrgID:  cached.OrgID.String(),
					Scopes: cached.Scopes,
				}
				r = WithOrg(r, &org)
				r = WithAPIKeyClaims(r, claims)

				if task, err := tasks.NewAPIKeyUpdateTask(cached.ID); err == nil {
					if _, err := enqueuer.Enqueue(task); err != nil {
						slog.Debug("failed to enqueue apikey update", "error", err)
					}
				}

				next.ServeHTTP(w, r)
				return
			}

			// L2: Database lookup
			var apiKey model.APIKey
			if err := db.Preload("Org").Where("key_hash = ? AND revoked_at IS NULL", keyHash).
				First(&apiKey).Error; err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid api key"})
				return
			}

			if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "api key expired"})
				return
			}

			if !apiKey.Org.Active {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "organization is inactive"})
				return
			}

			// Promote to cache
			keyCache.Set(keyHash, &cache.CachedAPIKey{
				ID:        apiKey.ID,
				OrgID:     apiKey.OrgID,
				Scopes:    apiKey.Scopes,
				ExpiresAt: apiKey.ExpiresAt,
			})

			claims := &APIKeyClaims{
				KeyID:  apiKey.ID.String(),
				OrgID:  apiKey.OrgID.String(),
				Scopes: apiKey.Scopes,
			}
			r = WithOrg(r, &apiKey.Org)
			r = WithAPIKeyClaims(r, claims)

			if task, err := tasks.NewAPIKeyUpdateTask(apiKey.ID); err == nil {
				if _, err := enqueuer.Enqueue(task); err != nil {
					slog.Debug("failed to enqueue apikey update", "error", err)
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// MultiAuth dispatches authentication based on the bearer token prefix:
// - "zira_sk_*" → API Key auth
// - everything else → Embedded JWT auth (RS256)
func MultiAuth(pubKey *rsa.PublicKey, issuer, audience string, db *gorm.DB, keyCache *cache.APIKeyCache, enqueuer enqueue.TaskEnqueuer) func(http.Handler) http.Handler {
	authMW := RequireAuth(pubKey, issuer, audience)
	apiKeyMW := APIKeyAuth(db, keyCache, enqueuer)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
				return
			}

			if strings.HasPrefix(token, "zira_sk_") {
				apiKeyMW(next).ServeHTTP(w, r)
			} else {
				authMW(next).ServeHTTP(w, r)
			}
		})
	}
}

// ResolveOrgFlexible resolves the org from context. If already set (by API key auth), it's a no-op.
// Otherwise, falls back to the X-Org-ID header for JWT-authenticated requests.
func ResolveOrgFlexible(db *gorm.DB) func(http.Handler) http.Handler {
	headerResolve := ResolveOrgFromHeader(db)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := OrgFromContext(r.Context()); ok {
				next.ServeHTTP(w, r)
				return
			}
			headerResolve(next).ServeHTTP(w, r)
		})
	}
}

// RequireAPIKeyScopeOrJWT enforces scope checking for API key auth.
// JWT-authenticated requests pass through unchecked (they use org-level roles).
func RequireAPIKeyScopeOrJWT(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// JWT-authenticated requests bypass scope checks
			if _, ok := AuthClaimsFromContext(r.Context()); ok {
				next.ServeHTTP(w, r)
				return
			}

			claims, ok := APIKeyClaimsFromContext(r.Context())
			if !ok {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient permissions"})
				return
			}

			for _, s := range claims.Scopes {
				if s == scope || s == "all" {
					next.ServeHTTP(w, r)
					return
				}
			}

			writeJSON(w, http.StatusForbidden, map[string]string{"error": "api key lacks required scope: " + scope})
		})
	}
}
