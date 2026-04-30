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

type testUserKey struct{}
type testOrgKey struct{}

func TestMiddleware_AppliesUserAndOrg(t *testing.T) {
	withTestSentry(t)

	SetUserExtractor(func(ctx context.Context) string {
		v, _ := ctx.Value(testUserKey{}).(string)
		return v
	})
	SetOrgExtractor(func(ctx context.Context) string {
		v, _ := ctx.Value(testOrgKey{}).(string)
		return v
	})

	var captured atomic.Pointer[sentrygo.Event]
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hub := sentrygo.GetHubFromContext(r.Context())
		if hub == nil {
			t.Fatal("expected hub on request context")
		}
		hub.WithScope(func(scope *sentrygo.Scope) {
			ev := sentrygo.NewEvent()
			scope.ApplyToEvent(ev, nil, hub.Client())
			captured.Store(ev)
		})
	})

	wrapped := Middleware()(handler)

	ctx := context.WithValue(context.Background(), testUserKey{}, "user-abc")
	ctx = context.WithValue(ctx, testOrgKey{}, "org-xyz")
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	wrapped.ServeHTTP(httptest.NewRecorder(), req)

	ev := captured.Load()
	if ev == nil {
		t.Fatal("expected event captured")
	}
	if ev.User.ID != "user-abc" {
		t.Fatalf("user.id = %q, want user-abc", ev.User.ID)
	}
	if ev.Tags["org_id"] != "org-xyz" {
		t.Fatalf("tag org_id = %q, want org-xyz", ev.Tags["org_id"])
	}
}

func TestMiddleware_OrgOnlyRequest(t *testing.T) {
	withTestSentry(t)

	SetUserExtractor(func(context.Context) string { return "" })
	SetOrgExtractor(func(ctx context.Context) string {
		v, _ := ctx.Value(testOrgKey{}).(string)
		return v
	})

	var captured atomic.Pointer[sentrygo.Event]
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hub := sentrygo.GetHubFromContext(r.Context())
		hub.WithScope(func(scope *sentrygo.Scope) {
			ev := sentrygo.NewEvent()
			scope.ApplyToEvent(ev, nil, hub.Client())
			captured.Store(ev)
		})
	})

	wrapped := Middleware()(handler)
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(
		context.WithValue(context.Background(), testOrgKey{}, "org-only"),
	)
	wrapped.ServeHTTP(httptest.NewRecorder(), req)

	ev := captured.Load()
	if ev == nil {
		t.Fatal("expected event captured")
	}
	if ev.User.ID != "" {
		t.Fatalf("user.id = %q, want empty", ev.User.ID)
	}
	if ev.Tags["org_id"] != "org-only" {
		t.Fatalf("tag org_id = %q, want org-only", ev.Tags["org_id"])
	}
}
