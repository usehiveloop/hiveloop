package handler_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestNotionProxy_EmployeeForwardsVersionedRequest(t *testing.T) {
	var captured struct {
		method        string
		path          string
		query         string
		auth          string
		providerKey   string
		connectionID  string
		contentType   string
		notionVersion string
		body          string
	}
	var mu sync.Mutex
	harness := newNotionProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.query = r.URL.RawQuery
		captured.auth = r.Header.Get("Authorization")
		captured.providerKey = r.Header.Get("Provider-Config-Key")
		captured.connectionID = r.Header.Get("Connection-Id")
		captured.contentType = r.Header.Get("Content-Type")
		captured.notionVersion = r.Header.Get("Notion-Version")
		captured.body = string(body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Remaining", "42")
		_, _ = w.Write([]byte(`{"object":"page","id":"page-1","url":"https://www.notion.so/page-1"}`))
	}))

	req := httptest.NewRequest(http.MethodPatch, "/internal/notion-proxy/"+harness.employeeID.String()+"/v1/pages/page-1?filter=value", bytes.NewReader([]byte(`{"archived":false}`)))
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Notion-Version", "2022-06-28")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "42" {
		t.Fatalf("rate limit header = %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if captured.method != http.MethodPatch || captured.path != "/proxy/v1/pages/page-1" || captured.query != "filter=value" {
		t.Fatalf("captured request = %+v", captured)
	}
	if captured.auth != "Bearer test-nango-secret" || captured.providerKey != harness.providerKey || captured.connectionID != "notion-nango-1" {
		t.Fatalf("captured nango headers = %+v", captured)
	}
	if captured.contentType != "application/json" || captured.notionVersion != "2022-06-28" {
		t.Fatalf("captured notion headers = %+v", captured)
	}
	if captured.body != `{"archived":false}` {
		t.Fatalf("body = %q", captured.body)
	}
}

func TestNotionProxy_SubagentUsesOwningEmployeeConnection(t *testing.T) {
	var connectionID string
	harness := newNotionProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectionID = r.Header.Get("Connection-Id")
		_, _ = w.Write([]byte(`ok`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/internal/notion-proxy/"+harness.subagentID.String()+"/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Notion-Version", "2022-06-28")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if connectionID != "notion-nango-1" {
		t.Fatalf("connection id = %q", connectionID)
	}
}

func TestNotionProxy_RejectsInvalidAndUnattachedRequests(t *testing.T) {
	var nangoCalls atomic.Int64
	harness := newNotionProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		{name: "invalid bearer", agentID: harness.employeeID, path: "/v1/users/me", token: "bad", want: http.StatusUnauthorized},
		{name: "standalone agent", agentID: harness.standaloneID, path: "/v1/users/me", token: harness.bridgeKey, want: http.StatusNotFound},
		{name: "invalid path", agentID: harness.employeeID, path: "/users/me", token: harness.bridgeKey, want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/internal/notion-proxy/"+tt.agentID.String()+tt.path, nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			req.Header.Set("Notion-Version", "2022-06-28")
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

func TestNotionProxy_RequiresActiveConnection(t *testing.T) {
	var nangoCalls atomic.Int64
	harness := newNotionProxyHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nangoCalls.Add(1)
	}))
	revokedAt := mustParseTime(t, "2026-05-17T00:00:00Z")
	if err := harness.db.Model(&model.InConnection{}).Where("id = ?", harness.connectionID).Update("revoked_at", revokedAt).Error; err != nil {
		t.Fatalf("revoke connection: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/internal/notion-proxy/"+harness.employeeID.String()+"/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+harness.bridgeKey)
	req.Header.Set("Notion-Version", "2022-06-28")
	rec := httptest.NewRecorder()
	harness.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no notion connection") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if got := nangoCalls.Load(); got != 0 {
		t.Fatalf("nango calls = %d", got)
	}
}
