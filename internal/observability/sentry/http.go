package sentry

import (
	"net/http"

	sentrygo "github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
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
				applyUserToScope(request.Context(), hub.Scope())
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
				applyUserToScope(request.Context(), hub.Scope())
			}
			next.ServeHTTP(writer, request)
		})
	}
}
