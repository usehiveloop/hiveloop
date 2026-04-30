package sentry

import (
	"context"
	"log/slog"

	sentryslog "github.com/getsentry/sentry-go/slog"
)

func WrapSlogHandler(base slog.Handler) slog.Handler {
	if !Enabled() {
		return base
	}
	sentryHandler := sentryslog.Option{
		EventLevel: []slog.Level{slog.LevelError},
		AddSource:  true,
	}.NewSentryHandler(context.Background())

	return &fanoutHandler{base: base, sentry: sentryHandler}
}

type fanoutHandler struct {
	base   slog.Handler
	sentry slog.Handler
}

func (f *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return f.base.Enabled(ctx, level) || f.sentry.Enabled(ctx, level)
}

func (f *fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	if f.base.Enabled(ctx, record.Level) {
		if err := f.base.Handle(ctx, record); err != nil {
			return err
		}
	}
	if f.sentry.Enabled(ctx, record.Level) {
		_ = f.sentry.Handle(ctx, record)
	}
	return nil
}

func (f *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &fanoutHandler{
		base:   f.base.WithAttrs(attrs),
		sentry: f.sentry.WithAttrs(attrs),
	}
}

func (f *fanoutHandler) WithGroup(name string) slog.Handler {
	return &fanoutHandler{
		base:   f.base.WithGroup(name),
		sentry: f.sentry.WithGroup(name),
	}
}
