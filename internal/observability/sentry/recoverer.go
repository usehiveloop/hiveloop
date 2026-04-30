package sentry

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

func Recoverer() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			ctx := request.Context()
			defer func(ctx context.Context, method, path string) {
				recovered := recover()
				if recovered == nil {
					return
				}
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}

				stack := debug.Stack()
				slog.ErrorContext(ctx, "panic recovered in HTTP handler",
					"panic", recovered,
					"method", method,
					"path", path,
					"stack", string(stack),
				)

				CaptureException(ctx, fmt.Errorf("panic in %s %s: %v", method, path, recovered))

				writer.WriteHeader(http.StatusInternalServerError)
			}(ctx, request.Method, request.URL.Path)

			next.ServeHTTP(writer, request)
		})
	}
}
