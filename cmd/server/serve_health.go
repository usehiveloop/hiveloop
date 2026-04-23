package main

import (
	"log/slog"
	"net/http"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// healthz is a public liveness probe. The response body is intentionally
// opaque so it cannot be used by unauthenticated callers to enumerate
// infrastructure or distinguish dependency states.
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// readyz is a public readiness probe. Failures are logged with full detail
// server-side but the response body is opaque — it only signals ready/not
// ready and never discloses which backing service (db, redis, etc.) caused
// the failure.
func readyz(database *gorm.DB, rc *redis.Client) http.HandlerFunc {
	const notReadyBody = `{"status":"not ready"}`

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		sqlDB, err := database.DB()
		if err != nil {
			slog.Warn("readiness check: db handle unavailable", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(notReadyBody))
			return
		}
		if err := sqlDB.Ping(); err != nil {
			slog.Warn("readiness check: db ping failed", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(notReadyBody))
			return
		}

		if err := rc.Ping(r.Context()).Err(); err != nil {
			slog.Warn("readiness check: redis ping failed", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(notReadyBody))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
