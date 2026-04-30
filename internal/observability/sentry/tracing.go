package sentry

import (
	"context"

	sentrygo "github.com/getsentry/sentry-go"
)

func StartSpan(ctx context.Context, op, description string) *sentrygo.Span {
	if !Enabled() {
		return nil
	}
	span := sentrygo.StartSpan(ctx, op)
	if description != "" {
		span.Description = description
	}
	return span
}

func SpanFromContext(ctx context.Context) *sentrygo.Span {
	if !Enabled() || ctx == nil {
		return nil
	}
	return sentrygo.SpanFromContext(ctx)
}

func FinishSpanWithError(span *sentrygo.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.Status = sentrygo.SpanStatusInternalError
		span.SetData("error", err.Error())
	} else if span.Status == 0 {
		span.Status = sentrygo.SpanStatusOK
	}
	span.Finish()
}
