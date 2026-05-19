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

func TestBugsinkProxy_EmployeeForwardsRawRequest(t *testing.T) {
	var captured struct {
		method       string
		path         string
		query        string
		auth         string
		providerKey  string
		connectionID string
		contentType  string
	}
	var mu sync.Mutex
	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.query = r.URL.RawQuery
		captured.auth = r.Header.Get("Authorization")
		captured.providerKey = r.Header.Get("Provider-Config-Key")
		captured.connectionID = r.Header.Get("Connection-Id")
		captured.contentType = r.Header.Get("Content-Type")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "99")
		_, _ = w.Write([]byte(`{"projects":[{"id":"1","name":"api"}]}`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/internal/bugsink-proxy/"+harness.employeeID.String()+"/api/canonical/0/projects/?cursor=abc", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type = %q", got)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "99" {
		t.Fatalf("rate limit header = %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if captured.method != http.MethodGet || captured.path != "/proxy/projects/" || captured.query != "cursor=abc" {
		t.Fatalf("captured request = %+v", captured)
	}
	if captured.auth != "Bearer test-nango-secret" || captured.providerKey == "" || captured.connectionID != "bugsink-nango-1" {
		t.Fatalf("captured nango headers = %+v", captured)
	}
}

func TestBugsinkProxy_SubagentUsesOwningEmployeeConnection(t *testing.T) {
	var connectionID string
	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectionID = r.Header.Get("Connection-Id")
		_, _ = w.Write([]byte(`ok`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/internal/bugsink-proxy/"+harness.subagentID.String()+"/api/canonical/0/issues/", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if connectionID != "bugsink-nango-1" {
		t.Fatalf("connection id = %q", connectionID)
	}
}

func TestBugsinkProxy_RejectsInvalidAndUnattachedRequests(t *testing.T) {
	var nangoCalls atomic.Int64
	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nangoCalls.Add(1)
		_, _ = w.Write([]byte(`ok`))
	}))

	tests := []struct {
		name    string
		agentID uuid.UUID
		path    string
		token   string
		want    int
	}{
		{name: "invalid bearer", agentID: harness.employeeID, path: "/api/canonical/0/projects/", token: "bad", want: http.StatusUnauthorized},
		{name: "standalone agent", agentID: harness.standaloneID, path: "/api/canonical/0/projects/", token: harness.bridgeKey, want: http.StatusNotFound},
		{name: "invalid path", agentID: harness.employeeID, path: "/api/other/", token: harness.bridgeKey, want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/internal/bugsink-proxy/"+tt.agentID.String()+tt.path, nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
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

func TestBugsinkProxy_UsesActiveOrgConnectionWithoutEmployeeAssignment(t *testing.T) {
	var nangoCalls atomic.Int64
	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nangoCalls.Add(1)
	}))
	if err := harness.db.Model(&model.Agent{}).Where("id = ?", harness.employeeID).Update("integrations", model.JSON{}).Error; err != nil {
		t.Fatalf("clear integrations: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/internal/bugsink-proxy/"+harness.employeeID.String()+"/api/canonical/0/projects/", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := nangoCalls.Load(); got != 1 {
		t.Fatalf("nango calls = %d", got)
	}
}

func TestBugsinkProxy_ForwardsPostAndUpstreamErrors(t *testing.T) {
	var capturedBody string
	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/internal/bugsink-proxy/"+harness.employeeID.String()+"/api/canonical/0/issues/", bytes.NewReader([]byte(`{"query":"x"}`)))
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if capturedBody != `{"query":"x"}` {
		t.Fatalf("body = %q", capturedBody)
	}
	if rec.Body.String() != `{"error":"forbidden"}` {
		t.Fatalf("response body = %q", rec.Body.String())
	}
}

func TestBugsinkProxy_NangoTransportFailureReturnsBadGateway(t *testing.T) {
	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	badHandler := handler.NewBugsinkProxyHandler(harness.db, testSymmetricKey(t), nango.NewClient("http://127.0.0.1:1", "test-nango-secret"))
	router := chi.NewRouter()
	router.Handle("/internal/bugsink-proxy/{agentID}/*", http.HandlerFunc(badHandler.Handle))

	req := httptest.NewRequest(http.MethodGet, "/internal/bugsink-proxy/"+harness.employeeID.String()+"/api/canonical/0/projects/", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBugsinkProxy_SentryCapturesSafeContext(t *testing.T) {
	transport := &bugsinkCaptureTransport{}
	if err := sentrygo.Init(sentrygo.ClientOptions{Dsn: "https://public@example.com/1", Transport: transport, EnableTracing: false}); err != nil {
		t.Fatalf("sentry init: %v", err)
	}
	t.Cleanup(func() { sentrygo.Flush(0) })

	harness := newBugsinkProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"token":"must-not-leak"}`))
	}))
	req := httptest.NewRequest(http.MethodGet, "/internal/bugsink-proxy/"+harness.employeeID.String()+"/api/canonical/0/projects/?secret=must-not-leak", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)
	sentrygo.Flush(0)

	event := transport.event.Load()
	if event == nil {
		t.Fatal("expected sentry event")
	}
	if got := event.Tags["bugsink_proxy"]; got != "true" {
		t.Fatalf("bugsink_proxy tag = %q", got)
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
