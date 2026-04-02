package goroutine

import (
	"log/slog"
	"runtime/debug"
)

// Go runs fn in a new goroutine with panic recovery.
// If fn panics, the panic value and stack trace are logged via slog.Error.
// This is intended for singleton background goroutines (cleanup loops, flushers, subscribers).
// For fan-out work (parallel queries, batch processing), use sourcegraph/conc pools instead.
func Go(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("goroutine panicked",
					"panic", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}
