package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/handler"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/nango"
)

type connTestHarness struct {
	db      *gorm.DB
	handler *handler.ConnectionHandler
	router  *chi.Mux
	nango   *httptest.Server
}

func newConnHarness(t *testing.T, nangoHandler http.Handler) *connTestHarness {
	t.Helper()
	db := connectTestDB(t)
	nangoSrv := httptest.NewServer(nangoHandler)
	t.Cleanup(nangoSrv.Close)

	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	h := handler.NewConnectionHandler(db, nangoClient, nil)

	r := chi.NewRouter()
	r.HandleFunc("/v1/connections/{id}/proxy/*", h.Proxy)
	r.Get("/v1/connections/{id}", h.Get)
	r.Delete("/v1/connections/{id}", h.Revoke)

	return &connTestHarness{
		db:      db,
		handler: h,
		router:  r,
		nango:   nangoSrv,
	}
}

func (h *connTestHarness) doConnRequest(t *testing.T, method, path string, body any, org *model.Org) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		bodyReader = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if org != nil {
		req = middleware.WithOrg(req, org)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func createTestIntegration(t *testing.T, db *gorm.DB, orgID uuid.UUID, provider string) model.Integration {
	t.Helper()
	integ := model.Integration{
		ID:          uuid.New(),
		OrgID:       orgID,
		UniqueKey:   fmt.Sprintf("%s-%s", provider, uuid.New().String()[:8]),
		Provider:    provider,
		DisplayName: provider + " test",
	}
	if err := db.Create(&integ).Error; err != nil {
		t.Fatalf("create integration: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", integ.ID).Delete(&model.Integration{})
	})
	return integ
}

func createTestConnection(t *testing.T, db *gorm.DB, orgID, integID uuid.UUID, nangoConnID string) model.Connection {
	t.Helper()
	conn := model.Connection{
		ID:                uuid.New(),
		OrgID:             orgID,
		IntegrationID:     integID,
		NangoConnectionID: nangoConnID,
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", conn.ID).Delete(&model.Connection{})
	})
	return conn
}

// capturedRequest stores details captured by the fake Nango proxy handler.
type capturedRequest struct {
	Method      string
	Path        string
	RawQuery    string
	ContentType string
	Body        []byte
	Headers     http.Header
}

// nangoProxyCapture returns an http.Handler that captures proxy requests at /proxy/*
// and responds with the given status code, content type, and body.
func nangoProxyCapture(captured *capturedRequest, mu *sync.Mutex, statusCode int, respContentType string, respBody []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		*captured = capturedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			RawQuery:    r.URL.RawQuery,
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
			Headers:     r.Header.Clone(),
		}
		mu.Unlock()

		if respContentType != "" {
			w.Header().Set("Content-Type", respContentType)
		}
		w.WriteHeader(statusCode)
		w.Write(respBody)
	})
}

// --------------------------------------------------------------------------
// Proxy tests
// --------------------------------------------------------------------------

func TestProxy_Success_GET(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex
	respBody := []byte(`{"repos": [{"name": "llmvault"}]}`)
	nangoHandler := nangoProxyCapture(&captured, &mu, http.StatusOK, "application/json", respBody)

	h := newConnHarness(t, nangoHandler)
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "github-app")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-conn-1")

	rr := h.doConnRequest(t, http.MethodGet, "/v1/connections/"+conn.ID.String()+"/proxy/user/repos", nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify response body passed through
	if rr.Body.String() != string(respBody) {
		t.Fatalf("expected body %q, got %q", string(respBody), rr.Body.String())
	}

	// Verify Nango received the correct request
	mu.Lock()
	defer mu.Unlock()
	if captured.Method != "GET" {
		t.Fatalf("expected GET, got %s", captured.Method)
	}
	if captured.Path != "/proxy/user/repos" {
		t.Fatalf("expected path /proxy/user/repos, got %s", captured.Path)
	}
	expectedKey := fmt.Sprintf("%s_%s", org.ID.String(), integ.UniqueKey)
	if captured.Headers.Get("Provider-Config-Key") != expectedKey {
		t.Fatalf("expected Provider-Config-Key=%s, got %s", expectedKey, captured.Headers.Get("Provider-Config-Key"))
	}
	if captured.Headers.Get("Connection-Id") != conn.NangoConnectionID {
		t.Fatalf("expected Connection-Id=%s, got %s", conn.NangoConnectionID, captured.Headers.Get("Connection-Id"))
	}
}

func TestProxy_Success_POST_WithBody(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex
	respBody := []byte(`{"token": "ghs_xxx", "expires_at": "2026-03-21T00:00:00Z"}`)
	nangoHandler := nangoProxyCapture(&captured, &mu, http.StatusCreated, "application/json", respBody)

	h := newConnHarness(t, nangoHandler)
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "github-app")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-conn-2")

	reqBody := map[string]any{
		"permissions": map[string]string{"contents": "read"},
	}

	rr := h.doConnRequest(t, http.MethodPost, "/v1/connections/"+conn.ID.String()+"/proxy/app/installations/12345/access_tokens", reqBody, &org)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify body was forwarded
	mu.Lock()
	defer mu.Unlock()
	if captured.Method != "POST" {
		t.Fatalf("expected POST, got %s", captured.Method)
	}
	if len(captured.Body) == 0 {
		t.Fatal("expected request body to be forwarded")
	}
	var forwardedBody map[string]any
	if err := json.Unmarshal(captured.Body, &forwardedBody); err != nil {
		t.Fatalf("unmarshal forwarded body: %v", err)
	}
	if _, ok := forwardedBody["permissions"]; !ok {
		t.Fatal("expected 'permissions' in forwarded body")
	}
}

func TestProxy_PreservesQueryParams(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex
	nangoHandler := nangoProxyCapture(&captured, &mu, http.StatusOK, "application/json", []byte(`[]`))

	h := newConnHarness(t, nangoHandler)
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "github-app")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-conn-3")

	rr := h.doConnRequest(t, http.MethodGet, "/v1/connections/"+conn.ID.String()+"/proxy/user/repos?per_page=10&sort=updated", nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if captured.RawQuery != "per_page=10&sort=updated" {
		t.Fatalf("expected query 'per_page=10&sort=updated', got %q", captured.RawQuery)
	}
}

func TestProxy_PreservesUpstreamStatusCode(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex
	nangoHandler := nangoProxyCapture(&captured, &mu, http.StatusNotFound, "application/json", []byte(`{"message": "Not Found"}`))

	h := newConnHarness(t, nangoHandler)
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "github-app")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-conn-4")

	rr := h.doConnRequest(t, http.MethodGet, "/v1/connections/"+conn.ID.String()+"/proxy/repos/nonexistent", nil, &org)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 from upstream, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestProxy_PreservesContentType(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex
	nangoHandler := nangoProxyCapture(&captured, &mu, http.StatusOK, "text/plain; charset=utf-8", []byte("hello"))

	h := newConnHarness(t, nangoHandler)
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "github-app")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-conn-5")

	rr := h.doConnRequest(t, http.MethodGet, "/v1/connections/"+conn.ID.String()+"/proxy/raw/file", nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("expected Content-Type 'text/plain; charset=utf-8', got %q", ct)
	}
}

func TestProxy_ConnectionNotFound(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex
	nangoHandler := nangoProxyCapture(&captured, &mu, http.StatusOK, "application/json", nil)

	h := newConnHarness(t, nangoHandler)
	org := createTestOrg(t, h.db)

	rr := h.doConnRequest(t, http.MethodGet, "/v1/connections/"+uuid.New().String()+"/proxy/anything", nil, &org)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestProxy_RevokedConnection(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex
	nangoHandler := nangoProxyCapture(&captured, &mu, http.StatusOK, "application/json", nil)

	h := newConnHarness(t, nangoHandler)
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "github-app")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-conn-6")

	now := time.Now()
	h.db.Model(&model.Connection{}).Where("id = ?", conn.ID).Update("revoked_at", &now)

	rr := h.doConnRequest(t, http.MethodGet, "/v1/connections/"+conn.ID.String()+"/proxy/anything", nil, &org)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for revoked connection, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestProxy_WrongOrg(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex
	nangoHandler := nangoProxyCapture(&captured, &mu, http.StatusOK, "application/json", nil)

	h := newConnHarness(t, nangoHandler)
	org1 := createTestOrg(t, h.db)
	org2 := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org1.ID, "github-app")
	conn := createTestConnection(t, h.db, org1.ID, integ.ID, "nango-conn-7")

	rr := h.doConnRequest(t, http.MethodGet, "/v1/connections/"+conn.ID.String()+"/proxy/anything", nil, &org2)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (wrong org), got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestProxy_MissingOrg(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex
	nangoHandler := nangoProxyCapture(&captured, &mu, http.StatusOK, "application/json", nil)

	h := newConnHarness(t, nangoHandler)

	rr := h.doConnRequest(t, http.MethodGet, "/v1/connections/"+uuid.New().String()+"/proxy/anything", nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestProxy_NangoTransportError(t *testing.T) {
	// Use a server that's immediately closed to cause a transport error
	nangoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	nangoSrv.Close()

	db := connectTestDB(t)
	nangoClient := nango.NewClient(nangoSrv.URL, "test-secret-key")
	connHandler := handler.NewConnectionHandler(db, nangoClient, nil)

	r := chi.NewRouter()
	r.HandleFunc("/v1/connections/{id}/proxy/*", connHandler.Proxy)

	org := createTestOrg(t, db)
	integ := createTestIntegration(t, db, org.ID, "github-app")
	conn := createTestConnection(t, db, org.ID, integ.ID, "nango-conn-8")

	req := httptest.NewRequest(http.MethodGet, "/v1/connections/"+conn.ID.String()+"/proxy/anything", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// Get enrichment tests
// --------------------------------------------------------------------------

func TestGet_WithProviderConfig(t *testing.T) {
	nangoConnResp := map[string]any{
		"connection_config": map[string]any{
			"installation_id": "12345",
			"app_id":          "67890",
			"jwtToken":        "eyJhbGciOiJSUzI1NiJ9.sensitive",
		},
		"metadata": map[string]any{
			"org_name": "acme-corp",
		},
		"provider": "github-app",
		"credentials": map[string]any{
			"access_token": "ghs_SECRET",
			"token_type":   "bearer",
		},
	}

	mux := chi.NewMux()
	mux.Get("/connection/{connectionId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(nangoConnResp)
	})
	mux.HandleFunc("/proxy/*", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	h := newConnHarness(t, mux)
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "github-app")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-get-1")

	rr := h.doConnRequest(t, http.MethodGet, "/v1/connections/"+conn.ID.String(), nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	pc, ok := resp["provider_config"].(map[string]any)
	if !ok {
		t.Fatal("expected provider_config in response")
	}

	// connection_config should be present
	if _, ok := pc["connection_config"]; !ok {
		t.Fatal("expected connection_config in provider_config")
	}
	connCfg := pc["connection_config"].(map[string]any)
	if connCfg["installation_id"] != "12345" {
		t.Fatalf("expected installation_id=12345, got %v", connCfg["installation_id"])
	}

	// jwtToken must be stripped from connection_config
	if _, ok := connCfg["jwtToken"]; ok {
		t.Fatal("jwtToken must not be present in connection_config")
	}

	// metadata should be present
	if _, ok := pc["metadata"]; !ok {
		t.Fatal("expected metadata in provider_config")
	}

	// provider should be present
	if pc["provider"] != "github-app" {
		t.Fatalf("expected provider=github-app, got %v", pc["provider"])
	}

	// credentials should be present with access_token
	creds, ok := pc["credentials"].(map[string]any)
	if !ok {
		t.Fatal("expected credentials in provider_config")
	}
	if creds["access_token"] != "ghs_SECRET" {
		t.Fatalf("expected access_token=ghs_SECRET, got %v", creds["access_token"])
	}
}

func TestGet_NangoFailure_StillReturnsConnection(t *testing.T) {
	mux := chi.NewMux()
	mux.Get("/connection/{connectionId}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal error"}`))
	})
	mux.HandleFunc("/proxy/*", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	h := newConnHarness(t, mux)
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "github-app")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-get-2")

	rr := h.doConnRequest(t, http.MethodGet, "/v1/connections/"+conn.ID.String(), nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// Connection data should still be present
	if resp["id"] != conn.ID.String() {
		t.Fatalf("expected id=%s, got %v", conn.ID.String(), resp["id"])
	}
	if resp["nango_connection_id"] != "nango-get-2" {
		t.Fatalf("expected nango_connection_id=nango-get-2, got %v", resp["nango_connection_id"])
	}

	// provider_config should be absent
	if _, ok := resp["provider_config"]; ok {
		t.Fatal("expected no provider_config when Nango fails")
	}
}

func TestProxy_WorksWithAnyProvider(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex
	respBody := []byte(`{"ok": true, "channels": []}`)
	nangoHandler := nangoProxyCapture(&captured, &mu, http.StatusOK, "application/json", respBody)

	h := newConnHarness(t, nangoHandler)
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "slack")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-conn-9")

	rr := h.doConnRequest(t, http.MethodGet, "/v1/connections/"+conn.ID.String()+"/proxy/conversations.list", nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	if rr.Body.String() != string(respBody) {
		t.Fatalf("expected body %q, got %q", string(respBody), rr.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	expectedKey := fmt.Sprintf("%s_%s", org.ID.String(), integ.UniqueKey)
	if captured.Headers.Get("Provider-Config-Key") != expectedKey {
		t.Fatalf("expected Provider-Config-Key=%s, got %s", expectedKey, captured.Headers.Get("Provider-Config-Key"))
	}
}
