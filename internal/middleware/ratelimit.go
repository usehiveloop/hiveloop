package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimit returns middleware that enforces per-org rate limiting.
//
// It uses the org's RateLimit field (requests per minute) from the request context.
// The org must be set on the context by OrgAuth middleware before this runs.
// Returns 429 with Retry-After header when the limit is exceeded.
func RateLimit() func(http.Handler) http.Handler {
	var mu sync.Mutex
	limiters := make(map[string]*rate.Limiter)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			org, ok := OrgFromContext(r.Context())
			if !ok {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "org not found in context"})
				return
			}

			orgID := org.ID.String()

			mu.Lock()
			limiter, exists := limiters[orgID]
			if !exists {
				// rate.Limit is events per second; org.RateLimit is per minute
				rps := rate.Limit(float64(org.RateLimit) / 60.0)
				burst := max(org.RateLimit/10, 1)
				limiter = rate.NewLimiter(rps, burst)
				limiters[orgID] = limiter
			}
			mu.Unlock()

			if !limiter.Allow() {
				retryAfter := time.Second
				w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
