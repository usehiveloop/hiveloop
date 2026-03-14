package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/model"
)

// identityRateLimitScript atomically checks and increments a rate limit counter.
// Returns 1 if allowed, 0 if limit exceeded.
// Sets TTL on first use so the window auto-expires.
var identityRateLimitScript = redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])

local current = redis.call("GET", key)
if current == false then
    redis.call("SET", key, 1, "PX", window_ms)
    return 1
end

local n = tonumber(current)
if n >= limit then
    return 0
end

redis.call("INCR", key)
return 1
`)

// IdentityRateLimit returns middleware that enforces identity-level shared rate limits.
// It looks up the credential's identity (if any), then checks each of its rate limits
// in Redis. Fails open if Redis is unavailable.
func IdentityRateLimit(rdb *redis.Client, db *gorm.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()

			// Look up credential's identity
			var cred model.Credential
			if err := db.WithContext(ctx).Select("identity_id").Where("id = ?", claims.CredentialID).First(&cred).Error; err != nil {
				// Can't find credential — let downstream handle it
				next.ServeHTTP(w, r)
				return
			}

			// Always set identity ID on context for downstream middleware (e.g. Audit)
			r = WithCredentialIdentityID(r, cred.IdentityID)

			if cred.IdentityID == nil {
				// No identity linked — skip identity rate limiting
				next.ServeHTTP(w, r)
				return
			}

			// Load identity rate limits
			var rateLimits []model.IdentityRateLimit
			if err := db.WithContext(ctx).Where("identity_id = ?", *cred.IdentityID).Find(&rateLimits).Error; err != nil {
				slog.Warn("identity_ratelimit: failed to load rate limits", "identity_id", cred.IdentityID, "error", err)
				next.ServeHTTP(w, r)
				return
			}

			if len(rateLimits) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			// Check each rate limit
			for _, rl := range rateLimits {
				key := fmt.Sprintf("pbrl:ident:%s:%s", cred.IdentityID.String(), rl.Name)
				windowMs := rl.Duration

				result, err := identityRateLimitScript.Run(ctx, rdb, []string{key}, rl.Limit, windowMs).Int()
				if err != nil {
					slog.Warn("identity_ratelimit: redis error, failing open", "key", key, "error", err)
					continue
				}

				if result == 0 {
					// Get TTL for Retry-After header
					ttl, _ := rdb.PTTL(ctx, key).Result()
					retryAfter := int(ttl / time.Millisecond / 1000)
					if retryAfter <= 0 {
						retryAfter = 1
					}
					w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
					writeJSON(w, http.StatusTooManyRequests, map[string]string{
						"error": fmt.Sprintf("identity rate limit exceeded: %s", rl.Name),
					})
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
