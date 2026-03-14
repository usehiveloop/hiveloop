package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// RequestLog returns middleware that writes a structured slog entry for every
// request. It captures method, path, status, latency, client IP, and the
// X-Request-Id set by Chi's RequestID middleware.
//
// Sensitive headers (Authorization, Cookie) are never logged.
func RequestLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			latency := time.Since(start)
			status := ww.Status()

			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int64("latency_ms", latency.Milliseconds()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.String("ip", clientIP(r)),
			}

			if reqID := chimw.GetReqID(r.Context()); reqID != "" {
				attrs = append(attrs, slog.String("request_id", reqID))
			}
			if org, ok := OrgFromContext(r.Context()); ok {
				attrs = append(attrs, slog.String("org_id", org.ID.String()))
			}
			if claims, ok := ClaimsFromContext(r.Context()); ok {
				attrs = append(attrs, slog.String("credential_id", claims.CredentialID))
			}

			level := slog.LevelInfo
			if status >= 500 {
				level = slog.LevelError
			} else if status >= 400 {
				level = slog.LevelWarn
			}

			logger.LogAttrs(r.Context(), level, "request", attrs...)
		})
	}
}

func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
