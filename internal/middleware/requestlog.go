package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/usehiveloop/hiveloop/internal/logging"
)

// RequestLog returns middleware that writes a single canonical structured
// slog entry per request capturing method, path, status, latency, client IP
// and the X-Request-Id from chimw.RequestID.
//
// It also seeds a contextual logger on the request context (with request_id
// as a base attribute), so downstream handlers can write correlated logs
// via logging.FromContext(ctx).Info(...) without re-supplying request_id.
//
// Sensitive headers (Authorization, Cookie) are never logged. Auth-derived
// attributes (org_id, user_id, credential_id) are appended to the canonical
// entry after the handler returns; downstream code may layer them onto the
// contextual logger via logging.WithAttrs as identity becomes known.
func RequestLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

			ctx := r.Context()
			reqID := chimw.GetReqID(ctx)
			scoped := logger
			if reqID != "" {
				scoped = scoped.With("request_id", reqID)
			}
			ctx = logging.WithLogger(ctx, scoped)
			r = r.WithContext(ctx)

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

			if reqID != "" {
				attrs = append(attrs, slog.String("request_id", reqID))
			}
			if org, ok := OrgFromContext(ctx); ok {
				attrs = append(attrs, slog.String("org_id", org.ID.String()))
			}
			if claims, ok := ClaimsFromContext(ctx); ok {
				attrs = append(attrs, slog.String("credential_id", claims.CredentialID))
			}

			level := slog.LevelInfo
			if status >= 500 {
				level = slog.LevelError
			} else if status >= 400 {
				level = slog.LevelWarn
			}

			logger.LogAttrs(ctx, level, "request", attrs...)
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
