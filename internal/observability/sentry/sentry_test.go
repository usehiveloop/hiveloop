package sentry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sentrygo "github.com/getsentry/sentry-go"
)

// withTestSentry forces Enabled()=true for the duration of t and configures a
// fresh global hub bound to a Transport that drops all events. The caller can
// inspect the hub's scope to assert on tags/user/contexts.
func withTestSentry(t *testing.T) {
	t.Helper()
	if err := sentrygo.Init(sentrygo.ClientOptions{
		Dsn:              "",
		Transport:        &nullTransport{},
		EnableTracing:    true,
		TracesSampleRate: 1.0,
	}); err != nil {
		t.Fatalf("sentry init: %v", err)
	}
	prev := initialized.Load()
	initialized.Store(true)
	t.Cleanup(func() { initialized.Store(prev) })
}

type nullTransport struct{}

func (*nullTransport) Configure(sentrygo.ClientOptions)      {}
func (*nullTransport) SendEvent(*sentrygo.Event)             {}
func (*nullTransport) SendEvents([]*sentrygo.Event)          {}
func (*nullTransport) Flush(time.Duration) bool              { return true }
func (*nullTransport) FlushWithContext(context.Context) bool { return true }
func (*nullTransport) Close()                                {}

func TestMiddleware_SetsUserOnHub(t *testing.T) {
	withTestSentry(t)

	SetUserExtractor(func(ctx context.Context) string {
		if v, ok := ctx.Value(testUserKey{}).(string); ok {
			return v
		}
		return ""
	})

	var captured atomic.Pointer[sentrygo.User]
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hub := sentrygo.GetHubFromContext(r.Context())
		if hub == nil {
			t.Fatal("expected hub on request context")
		}
		hub.WithScope(func(scope *sentrygo.Scope) {
			ev := sentrygo.NewEvent()
			scope.ApplyToEvent(ev, nil, hub.Client())
			captured.Store(&ev.User)
		})
	})

	wrapped := Middleware()(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), testUserKey{}, "user:abc-123"))
	wrapped.ServeHTTP(httptest.NewRecorder(), req)

	user := captured.Load()
	if user == nil {
		t.Fatal("expected user to be captured")
	}
	if user.ID != "user:abc-123" {
		t.Fatalf("expected user.ID=user:abc-123, got %q", user.ID)
	}
}

func TestMiddleware_NoExtractor_NoUser(t *testing.T) {
	withTestSentry(t)

	userExtractor.Store(nil)

	var captured atomic.Pointer[sentrygo.User]
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hub := sentrygo.GetHubFromContext(r.Context())
		hub.WithScope(func(scope *sentrygo.Scope) {
			ev := sentrygo.NewEvent()
			scope.ApplyToEvent(ev, nil, hub.Client())
			captured.Store(&ev.User)
		})
	})

	wrapped := Middleware()(handler)
	wrapped.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	user := captured.Load()
	if user != nil && user.ID != "" {
		t.Fatalf("expected empty user, got %q", user.ID)
	}
}

type testUserKey struct{}
