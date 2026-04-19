package goroutine

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	posthogobs "github.com/ziraloop/ziraloop/internal/observability/posthog"
)

// Go runs fn in a new goroutine with panic recovery.
// If fn panics, the panic value and stack trace are logged via slog.Error
// and explicitly captured to PostHog (so the exception has a full, untruncated
// stack trace rather than the shallow one from the slog capture handler).
// This is intended for singleton background goroutines (cleanup loops, flushers, subscribers).
// For fan-out work (parallel queries, batch processing), use sourcegraph/conc pools instead.
func Go(fn func()) {
	go func() {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}
			stack := debug.Stack()
			slog.Error("goroutine panicked",
				"panic", recovered,
				"stack", string(stack),
			)
			posthogobs.CaptureException(posthogobs.Default(), context.Background(),
				fmt.Sprintf("goroutine panic: %v", recovered),
				fmt.Sprintf("%v\n\n%s", recovered, stack),
			)
		}()
		fn()
	}()
}
