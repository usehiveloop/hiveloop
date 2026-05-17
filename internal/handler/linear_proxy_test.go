package handler_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	sentrygo "github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

func TestLinearProxy_EmployeeForwardsGraphQLRequest(t *testing.T) {
	var captured struct {
		method       string
		path         string
		auth         string
		providerKey  string
		connectionID string
		contentType  string
		body         string
	}
	var mu sync.Mutex
	harness := newLinearProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.auth = r.Header.Get("Authorization")
		captured.providerKey = r.Header.Get("Provider-Config-Key")
		captured.connectionID = r.Header.Get("Connection-Id")
		captured.contentType = r.Header.Get("Content-Type")
		captured.body = string(body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "77")
		_, _ = w.Write([]byte(`{"data":{"viewer":{"id":"user_1"}}}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/internal/linear-proxy/"+harness.employeeID.String(), bytes.NewReader([]byte(`{"query":"query Viewer { viewer { id } }"}`)))
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type = %q", got)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "77" {
		t.Fatalf("rate limit header = %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if captured.method != http.MethodPost || captured.path != "/proxy/graphql" {
		t.Fatalf("captured request = %+v", captured)
	}
	if captured.auth != "Bearer test-nango-secret" || captured.providerKey != harness.providerKey || captured.connectionID != "linear-nango-1" {
		t.Fatalf("captured nango headers = %+v", captured)
	}
	if captured.contentType != "application/json" || captured.body != `{"query":"query Viewer { viewer { id } }"}` {
		t.Fatalf("captured body/content-type = %+v", captured)
	}
}

func TestLinearProxy_SubagentUsesOwningEmployeeProfile(t *testing.T) {
	var connectionID string
	harness := newLinearProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectionID = r.Header.Get("Connection-Id")
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/internal/linear-proxy/"+harness.subagentID.String(), bytes.NewReader([]byte(`{"query":"query { viewer { id } }"}`)))
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if connectionID != "linear-nango-1" {
		t.Fatalf("connection id = %q", connectionID)
	}
}

func TestLinearProxy_RejectsInvalidAndUnattachedRequests(t *testing.T) {
	var nangoCalls atomic.Int64
	harness := newLinearProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nangoCalls.Add(1)
		_, _ = w.Write([]byte(`ok`))
	}))

	tests := []struct {
		name    string
		agentID uuid.UUID
		token   string
		want    int
	}{
		{name: "invalid bearer", agentID: harness.employeeID, token: "bad", want: http.StatusUnauthorized},
		{name: "standalone agent", agentID: harness.standaloneID, token: harness.bridgeKey, want: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/internal/linear-proxy/"+tt.agentID.String(), bytes.NewReader([]byte(`{"query":"query { viewer { id } }"}`)))
			req.Header.Set("Authorization", "Bearer "+tt.token)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			harness.router.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
	if got := nangoCalls.Load(); got != 0 {
		t.Fatalf("nango calls = %d", got)
	}
}

func TestLinearProxy_RequiresActiveLinearProfile(t *testing.T) {
	var nangoCalls atomic.Int64
	harness := newLinearProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nangoCalls.Add(1)
	}))
	if err := harness.db.Model(&model.AgentProfile{}).Where("id = ?", harness.profileID).Update("status", "pending").Error; err != nil {
		t.Fatalf("disable profile: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/linear-proxy/"+harness.employeeID.String(), bytes.NewReader([]byte(`{"query":"query { viewer { id } }"}`)))
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := nangoCalls.Load(); got != 0 {
		t.Fatalf("nango calls = %d", got)
	}
}

func TestLinearProxy_IgnoresRevokedConnection(t *testing.T) {
	var nangoCalls atomic.Int64
	harness := newLinearProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nangoCalls.Add(1)
	}))
	revokedAt := mustParseTime(t, "2026-05-17T00:00:00Z")
	if err := harness.db.Model(&model.InConnection{}).Where("id = ?", harness.connectionID).Update("revoked_at", revokedAt).Error; err != nil {
		t.Fatalf("revoke connection: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/linear-proxy/"+harness.employeeID.String(), bytes.NewReader([]byte(`{"query":"query { viewer { id } }"}`)))
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := nangoCalls.Load(); got != 0 {
		t.Fatalf("nango calls = %d", got)
	}
}

func TestLinearProxy_UpstreamErrorPassesThrough(t *testing.T) {
	harness := newLinearProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"message":"forbidden"}]}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/internal/linear-proxy/"+harness.employeeID.String(), bytes.NewReader([]byte(`{"query":"mutation { issueCreate(input:{}) { success } }"}`)))
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"errors":[{"message":"forbidden"}]}` {
		t.Fatalf("response body = %q", rec.Body.String())
	}
}

func TestLinearProxy_NangoTransportFailureReturnsBadGateway(t *testing.T) {
	harness := newLinearProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	badHandler := handler.NewLinearProxyHandler(harness.db, testSymmetricKey(t), nango.NewClient("http://127.0.0.1:1", "test-nango-secret"))
	router := chi.NewRouter()
	router.Post("/internal/linear-proxy/{agentID}", badHandler.Handle)

	req := httptest.NewRequest(http.MethodPost, "/internal/linear-proxy/"+harness.employeeID.String(), bytes.NewReader([]byte(`{"query":"query { viewer { id } }"}`)))
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLinearProxy_SentryCapturesSafeContext(t *testing.T) {
	transport := &bugsinkCaptureTransport{}
	if err := sentrygo.Init(sentrygo.ClientOptions{Dsn: "https://public@example.com/1", Transport: transport, EnableTracing: false}); err != nil {
		t.Fatalf("sentry init: %v", err)
	}
	t.Cleanup(func() { sentrygo.Flush(0) })

	harness := newLinearProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"token":"must-not-leak"}`))
	}))
	req := httptest.NewRequest(http.MethodPost, "/internal/linear-proxy/"+harness.employeeID.String()+"?secret=must-not-leak", bytes.NewReader([]byte(`{"token":"must-not-leak"}`)))
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)
	sentrygo.Flush(0)

	event := transport.event.Load()
	if event == nil {
		t.Fatal("expected sentry event")
	}
	if got := event.Tags["linear_proxy"]; got != "true" {
		t.Fatalf("linear_proxy tag = %q", got)
	}
	if got := event.Tags["http.status_code"]; got != "403" {
		t.Fatalf("status tag = %q", got)
	}
	exceptionValues := make([]string, 0, len(event.Exception))
	for _, ex := range event.Exception {
		exceptionValues = append(exceptionValues, ex.Value)
	}
	blob, _ := json.Marshal(map[string]any{
		"message":    event.Message,
		"tags":       event.Tags,
		"exceptions": exceptionValues,
	})
	if strings.Contains(string(blob), "secret=must-not-leak") || strings.Contains(string(blob), `{"token":"must-not-leak"}`) || strings.Contains(string(blob), harness.bridgeKey) {
		t.Fatalf("sentry event leaked sensitive request context: %s", string(blob))
	}
}
