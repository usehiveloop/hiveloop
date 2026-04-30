package sentry

import (
	"context"
	"sync/atomic"

	sentrygo "github.com/getsentry/sentry-go"
)

type UserExtractor func(ctx context.Context) string

var userExtractor atomic.Pointer[UserExtractor]

func SetUserExtractor(fn UserExtractor) {
	if fn == nil {
		return
	}
	userExtractor.Store(&fn)
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

func applyUserToScope(ctx context.Context, scope *sentrygo.Scope) {
	id := resolveUserID(ctx)
	if id == "" {
		return
	}
	scope.SetUser(sentrygo.User{ID: id})
}
