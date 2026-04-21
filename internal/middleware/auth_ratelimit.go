package middleware

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/usehiveloop/hiveloop/internal/goroutine"
)

type ipLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// AuthRateLimit returns middleware that enforces per-IP rate limiting on auth
// endpoints. It uses an in-memory map keyed by client IP (from chi's RealIP
// middleware). Stale entries are evicted every 5 minutes. The cleanup goroutine
// stops when ctx is cancelled.
func AuthRateLimit(ctx context.Context, rps float64, burst int) func(http.Handler) http.Handler {
	var mu sync.Mutex
	entries := make(map[string]*ipLimiterEntry)

	// Background cleanup of stale entries.
	goroutine.Go(func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mu.Lock()
				cutoff := time.Now().Add(-10 * time.Minute)
				for ip, e := range entries {
					if e.lastSeen.Before(cutoff) {
						delete(entries, ip)
					}
				}
				mu.Unlock()
			}
		}
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr

			mu.Lock()
			e, exists := entries[ip]
			if !exists {
				e = &ipLimiterEntry{
					limiter:  rate.NewLimiter(rate.Limit(rps), burst),
					lastSeen: time.Now(),
				}
				entries[ip] = e
			}
			e.lastSeen = time.Now()
			mu.Unlock()

			if !e.limiter.Allow() {
				w.Header().Set("Retry-After", strconv.Itoa(1))
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many requests"})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
