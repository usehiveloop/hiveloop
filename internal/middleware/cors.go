package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

// devFallbackOrigin is the default origin used when the server is running in
// development/test and no explicit allowlist was provided. This matches the
// Next.js dev server port configured in apps/web/package.json.
const devFallbackOrigin = "http://localhost:30112"

// isDevEnvironment reports whether the given environment string corresponds to
// a non-production environment where a permissive CORS fallback is acceptable.
func isDevEnvironment(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "", "dev", "development", "local", "test", "testing":
		return true
	default:
		return false
	}
}

// CORS returns middleware that allows cross-origin requests from the specified
// origins.
//
// Behavior when allowedOrigins is empty:
//   - In a development/test environment, the middleware falls back to a single
//     localhost origin and logs a warning. This avoids the historical behavior
//     of emitting a wildcard `*` which would accept requests from any site.
//   - In any other environment the factory returns an error so the server
//     refuses to start with an insecure CORS configuration.
//
// The returned middleware never emits `Access-Control-Allow-Origin: *` and
// never pairs `Access-Control-Allow-Credentials: true` with a wildcard origin.
func CORS(env string, allowedOrigins []string) (func(http.Handler) http.Handler, error) {
	return newCORS(env, allowedOrigins, false)
}

// AdminCORS returns a stricter CORS middleware for authenticated-only admin
// routes. When allowedOrigins is empty the returned middleware does NOT emit
// any CORS headers — admin endpoints are not expected to be consumed from
// arbitrary browsers. Preflight OPTIONS requests still receive a 204 to avoid
// breaking legitimate same-origin tooling.
func AdminCORS(allowedOrigins []string) func(http.Handler) http.Handler {
	mw, _ := newCORS("", allowedOrigins, true)
	return mw
}

func newCORS(env string, allowedOrigins []string, admin bool) (func(http.Handler) http.Handler, error) {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		allowed[o] = true
	}

	if len(allowed) == 0 {
		if admin {
			slog.Warn("admin CORS allowlist is empty — admin endpoints will reject all cross-origin browser requests")
		} else if isDevEnvironment(env) {
			allowed[devFallbackOrigin] = true
			slog.Warn("CORS_ORIGINS is empty — falling back to development default",
				"origin", devFallbackOrigin,
				"environment", env,
			)
		} else {
			return nil, fmt.Errorf("CORS_ORIGINS must be set in %q environment: refusing to start with a wildcard CORS policy", env)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Org-ID, Last-Event-ID, Cache-Control")
				w.Header().Set("Access-Control-Max-Age", "300")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}, nil
}
