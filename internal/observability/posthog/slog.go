package posthog

import (
	"context"
	"log/slog"

	ph "github.com/posthog/posthog-go"
)

// WrapSlogHandler mirrors every slog record at level Error or above to PostHog
// exception tracking. The returned handler forwards records to the supplied
// base handler unchanged, so normal structured logging keeps working.
//
// Records without an identifiable user/org/request are tagged with the
// "system" distinct ID so they still appear in the PostHog UI. This way
// background goroutines and worker tasks that don't have a request context
// are not silently dropped.
func WrapSlogHandler(base slog.Handler, client ph.Client) slog.Handler {
	if client == nil {
		return base
	}
	return ph.NewSlogCaptureHandler(
		base,
		client,
		ph.WithMinCaptureLevel(slog.LevelError),
		ph.WithDistinctIDFn(func(ctx context.Context, _ slog.Record) string {
			if id := DistinctID(ctx); id != "" {
				return id
			}
			return "system"
		}),
	)
}
