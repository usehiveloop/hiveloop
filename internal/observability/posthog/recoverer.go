package posthog

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	ph "github.com/posthog/posthog-go"
)

// Recoverer is a drop-in replacement for chi's built-in Recoverer middleware
// that also captures the panic value and stack trace to PostHog. It mirrors
// the behavior of chi middleware.Recoverer — returning a 500 on panic while
// letting http.ErrAbortHandler propagate — and adds explicit error capture
// on top.
//
// Safe to use with a nil client; when client is nil it behaves identically
// to chi's Recoverer without any capture.
func Recoverer(client ph.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// Capture the request context eagerly — once the panic fires the
			// request object is still valid, but passing ctx to the deferred
			// function makes the propagation explicit and keeps contextcheck
			// happy.
			ctx := request.Context()
			defer func(ctx context.Context, method, path string) {
				recovered := recover()
				if recovered == nil {
					return
				}
				// Re-panic on http.ErrAbortHandler so the server can abort
				// cleanly — matches chi middleware.Recoverer behavior.
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}

				stack := debug.Stack()
				description := fmt.Sprintf("%v\n\n%s", recovered, stack)

				slog.ErrorContext(ctx, "panic recovered in HTTP handler",
					"panic", recovered,
					"method", method,
					"path", path,
					"stack", string(stack),
				)

				if client != nil {
					CaptureException(client, ctx,
						fmt.Sprintf("panic: %v", recovered),
						description,
					)
				}

				writer.WriteHeader(http.StatusInternalServerError)
			}(ctx, request.Method, request.URL.Path)

			next.ServeHTTP(writer, request)
		})
	}
}
