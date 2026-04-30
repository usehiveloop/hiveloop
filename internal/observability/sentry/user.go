package sentry

import (
	"context"
	"sync/atomic"

	sentrygo "github.com/getsentry/sentry-go"
)

type StringExtractor func(ctx context.Context) string

var (
	userExtractor atomic.Pointer[StringExtractor]
	orgExtractor  atomic.Pointer[StringExtractor]
)

func SetUserExtractor(fn StringExtractor) {
	if fn == nil {
		return
	}
	userExtractor.Store(&fn)
}

func SetOrgExtractor(fn StringExtractor) {
	if fn == nil {
		return
	}
	orgExtractor.Store(&fn)
}

func resolveUserID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	extractor := userExtractor.Load()
	if extractor == nil {
		return ""
	}
	return (*extractor)(ctx)
}

func resolveOrgID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	extractor := orgExtractor.Load()
	if extractor == nil {
		return ""
	}
	return (*extractor)(ctx)
}

func applyAttribution(ctx context.Context, scope *sentrygo.Scope) {
	if id := resolveUserID(ctx); id != "" {
		scope.SetUser(sentrygo.User{ID: id})
	}
	if id := resolveOrgID(ctx); id != "" {
		scope.SetTag("org_id", id)
	}
}
