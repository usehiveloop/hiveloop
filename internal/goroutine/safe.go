package goroutine

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	posthogobs "github.com/usehiveloop/hiveloop/internal/observability/posthog"
)

// Go runs fn in a new goroutine with panic recovery.
//
// ctx is passed into fn so background work can participate in parent
// cancellation and carry request-scoped values (service name, org id, etc.)
// into any panic capture. Callers that genuinely have no context available
// should pass context.Background() explicitly so the omission is visible at
// the call site rather than hidden in a closure.
//
// If fn panics, the panic value and stack trace are logged via slog.Error
// and explicitly captured to PostHog (so the exception has a full,
// untruncated stack trace rather than the shallow one from the slog capture
// handler).
//
// Intended for singleton background goroutines (cleanup loops, flushers,
// subscribers). For fan-out work (parallel queries, batch processing), use
// sourcegraph/conc pools instead.
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
			posthogobs.CaptureException(posthogobs.Default(), ctx,
				fmt.Sprintf("goroutine panic: %v", recovered),
				fmt.Sprintf("%v\n\n%s", recovered, stack),
			)
		}()
		fn(ctx)
	}()
}
