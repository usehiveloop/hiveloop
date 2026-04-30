package goroutine

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	sentryobs "github.com/usehiveloop/hiveloop/internal/observability/sentry"
)

// Go runs fn in a new goroutine with panic recovery. ctx propagates parent
// cancellation and carries request-scoped values into any panic capture.
//
// Intended for singleton background goroutines (cleanup loops, flushers,
// subscribers). For fan-out work, use sourcegraph/conc pools instead.
func Go(ctx context.Context, fn func(context.Context)) {
	go func() {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			stack := debug.Stack()
			slog.ErrorContext(ctx, "goroutine panicked",
				"panic", recovered,
				"stack", string(stack),
			)
			sentryobs.CaptureException(ctx, fmt.Errorf("goroutine panic: %v\n\n%s", recovered, stack))
		}()
		fn(ctx)
	}()
}
