package sentry

import (
	"fmt"
	"net/http"

	sentrygo "github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func Middleware() func(http.Handler) http.Handler {
	if !Enabled() {
		return func(next http.Handler) http.Handler { return next }
	}

	sentryHandler := sentryhttp.New(sentryhttp.Options{
		Repanic:         true,
		WaitForDelivery: false,
	})

	return func(next http.Handler) http.Handler {
		enriched := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if hub := sentrygo.GetHubFromContext(request.Context()); hub != nil {
				hub.Scope().SetTag("http.method", request.Method)
				applyAttribution(request.Context(), hub.Scope())
			}
			next.ServeHTTP(writer, request)
		})
		return sentryHandler.Handle(enriched)
	}
}

// UserMiddleware re-applies the user extractor; install AFTER auth so the
// resolved user/org id attributes events captured later in the request.
func UserMiddleware() func(http.Handler) http.Handler {
	if !Enabled() {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if hub := sentrygo.GetHubFromContext(request.Context()); hub != nil {
				applyAttribution(request.Context(), hub.Scope())
			}
			next.ServeHTTP(writer, request)
		})
	}
}

// Capture5xxResponses captures handler-returned server errors that do not
// panic and may not have an explicit error log at the call site.
func Capture5xxResponses() func(http.Handler) http.Handler {
	if !Enabled() {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			wrapped := chimw.NewWrapResponseWriter(writer, request.ProtoMajor)
			next.ServeHTTP(wrapped, request)

			status := wrapped.Status()
			if status < http.StatusInternalServerError {
				return
			}
			if request.URL != nil && (request.URL.Path == "/healthz" || request.URL.Path == "/readyz") {
				return
			}
			path := ""
			if request.URL != nil {
				path = request.URL.Path
			}
			hub := hubFromContext(request.Context())
			hub.WithScope(func(scope *sentrygo.Scope) {
				scope.SetTag("http.status_code", fmt.Sprintf("%d", status))
				scope.SetTag("http.method", request.Method)
				if path != "" {
					scope.SetTag("http.route", path)
				}
				hub.CaptureException(fmt.Errorf("http %d response from %s %s", status, request.Method, path))
			})
		})
	}
}
