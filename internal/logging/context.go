package logging

import (
	"context"
	"log/slog"

	"github.com/usehivy/hivy/internal/observability/sentry"
)

// loggerKey is the context key used to attach a contextual *slog.Logger.
type loggerKey struct{}

// FromContext returns the *slog.Logger attached to ctx, or slog.Default() if
// none is set. Use this everywhere a context is available so that request /
// task scoped attributes (request_id, user_id, org_id, task_id, ...) are
// included automatically without re-supplying them on every call.
//
// Callers should prefer:
//
//	logging.FromContext(ctx).Info("user registered", "user_id", id)
//
// over:
//
//	slog.Info("user registered", "request_id", reqID, "user_id", id)
func FromContext(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return slog.Default()
	}
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

// WithLogger returns a copy of ctx that carries l as the contextual logger.
// Use this in middleware or task entry points to seed the contextual logger
// with request/task scoped attributes.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerKey{}, l)
}

// WithAttrs returns a copy of ctx whose contextual logger has the given
// attributes appended. The original logger in ctx is not modified.
//
//	ctx = logging.WithAttrs(ctx, "task_id", id, "trigger_id", trig.ID)
//	logging.FromContext(ctx).Info("starting") // includes task_id + trigger_id
func WithAttrs(ctx context.Context, attrs ...any) context.Context {
	if len(attrs) == 0 {
		return ctx
	}
	return WithLogger(ctx, FromContext(ctx).With(attrs...))
}

// Capture sends err to Sentry without emitting a log line. Use this for
// errors that are worth tracking (deduped, alertable) but not worth log
// noise — typically expected failures at internal boundaries that the caller
// has already wrapped and is about to return.
//
// For errors that should both alert *and* be visible in the log stream
// (genuine boundary errors at request/task entry), use slog.Error / the
// contextual logger — those already fan out to Sentry via the slog handler
// configured in observability/sentry.WrapSlogHandler.
//
// Capture is a no-op when err is nil or Sentry is not initialized.
func Capture(ctx context.Context, err error) {
	if err == nil {
		return
	}
	sentry.CaptureException(ctx, err)
}

// CaptureWithFields sends err to Sentry with additional structured context.
// The fields are also folded into the error message by Sentry's context payload,
// so callers can correlate expected boundary failures without noisy logs.
func CaptureWithFields(ctx context.Context, err error, fields map[string]any) {
	if err == nil {
		return
	}
	sentry.CaptureExceptionWithFields(ctx, err, fields)
}

// CaptureMessage sends a freeform message to Sentry without emitting a log
// line. Use sparingly — prefer wrapping an error and calling Capture.
func CaptureMessage(ctx context.Context, msg string) {
	if msg == "" {
		return
	}
	sentry.CaptureMessage(ctx, msg)
}
