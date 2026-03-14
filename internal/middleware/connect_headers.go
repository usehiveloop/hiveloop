package middleware

import (
	"fmt"
	"net/http"
	"strings"
)

// ConnectSecurityHeaders returns middleware that sets security headers
// for /connect/* routes, including dynamic CSP frame-ancestors from
// the session's allowed_origins.
func ConnectSecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")

			// Dynamic CSP frame-ancestors from session
			sess, ok := ConnectSessionFromContext(r.Context())
			if ok && len(sess.AllowedOrigins) > 0 {
				ancestors := strings.Join([]string(sess.AllowedOrigins), " ")
				w.Header().Set("Content-Security-Policy",
					fmt.Sprintf("frame-ancestors 'self' %s", ancestors))
			} else {
				w.Header().Set("X-Frame-Options", "DENY")
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ConnectCORS returns middleware that sets CORS headers dynamically
// based on the connect session's allowed origins. It must run after
// ConnectSessionAuth so the session is on context.
func ConnectCORS() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			sess, ok := ConnectSessionFromContext(r.Context())
			if ok && containsOrigin(sess.AllowedOrigins, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Max-Age", "300")
				w.Header().Set("Vary", "Origin")
			}

			next.ServeHTTP(w, r)
		})
	}
}
