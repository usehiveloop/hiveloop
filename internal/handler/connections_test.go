package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	r.Post("/v1/connections/{id}/token", h.RetrieveToken)
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
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
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

// nangoGetConnectionHandler returns an http.Handler that responds to
// GET /connection/{connectionId} with the given credentials map.
func nangoGetConnectionHandler(creds map[string]any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"credentials": creds,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
}

// --------------------------------------------------------------------------
// POST /v1/connections/{id}/token — RetrieveToken
// --------------------------------------------------------------------------

func TestRetrieveToken_Success(t *testing.T) {
	creds := map[string]any{
		"type":         "APP",
		"access_token": "ghs_test_token_abc123",
		"expires_at":   "2026-03-20T00:00:00Z",
	}
	h := newConnHarness(t, nangoGetConnectionHandler(creds))
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "github-app")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-conn-1")

	rr := h.doConnRequest(t, http.MethodPost, "/v1/connections/"+conn.ID.String()+"/token", nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["access_token"] != "ghs_test_token_abc123" {
		t.Fatalf("expected access_token=ghs_test_token_abc123, got %v", resp["access_token"])
	}
	if resp["token_type"] != "APP" {
		t.Fatalf("expected token_type=APP, got %v", resp["token_type"])
	}
	if resp["expires_at"] != "2026-03-20T00:00:00Z" {
		t.Fatalf("expected expires_at=2026-03-20T00:00:00Z, got %v", resp["expires_at"])
	}
	if resp["provider"] != "github-app" {
		t.Fatalf("expected provider=github-app, got %v", resp["provider"])
	}
	if resp["connection_id"] != conn.ID.String() {
		t.Fatalf("expected connection_id=%s, got %v", conn.ID.String(), resp["connection_id"])
	}
}

func TestRetrieveToken_UnsupportedProvider(t *testing.T) {
	h := newConnHarness(t, nangoGetConnectionHandler(nil))
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "slack")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-conn-2")

	rr := h.doConnRequest(t, http.MethodPost, "/v1/connections/"+conn.ID.String()+"/token", nil, &org)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["error"] == "" {
		t.Fatal("expected error message in response")
	}
}

func TestRetrieveToken_ConnectionNotFound(t *testing.T) {
	h := newConnHarness(t, nangoGetConnectionHandler(nil))
	org := createTestOrg(t, h.db)

	rr := h.doConnRequest(t, http.MethodPost, "/v1/connections/"+uuid.New().String()+"/token", nil, &org)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestRetrieveToken_RevokedConnection(t *testing.T) {
	h := newConnHarness(t, nangoGetConnectionHandler(nil))
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "github-app")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-conn-3")

	// Revoke the connection
	now := time.Now()
	h.db.Model(&model.Connection{}).Where("id = ?", conn.ID).Update("revoked_at", &now)

	rr := h.doConnRequest(t, http.MethodPost, "/v1/connections/"+conn.ID.String()+"/token", nil, &org)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for revoked connection, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestRetrieveToken_WrongOrg(t *testing.T) {
	h := newConnHarness(t, nangoGetConnectionHandler(nil))
	org1 := createTestOrg(t, h.db)
	org2 := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org1.ID, "github-app")
	conn := createTestConnection(t, h.db, org1.ID, integ.ID, "nango-conn-4")

	// Try to retrieve token from org2 — should not see org1's connection
	rr := h.doConnRequest(t, http.MethodPost, "/v1/connections/"+conn.ID.String()+"/token", nil, &org2)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (wrong org), got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestRetrieveToken_MissingOrg(t *testing.T) {
	h := newConnHarness(t, nangoGetConnectionHandler(nil))

	rr := h.doConnRequest(t, http.MethodPost, "/v1/connections/"+uuid.New().String()+"/token", nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestRetrieveToken_NangoError(t *testing.T) {
	nangoErr := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal"}`))
	})
	h := newConnHarness(t, nangoErr)
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "github-app")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-conn-5")

	rr := h.doConnRequest(t, http.MethodPost, "/v1/connections/"+conn.ID.String()+"/token", nil, &org)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestRetrieveToken_NoAccessTokenInResponse(t *testing.T) {
	creds := map[string]any{
		"type": "APP",
	}
	h := newConnHarness(t, nangoGetConnectionHandler(creds))
	org := createTestOrg(t, h.db)
	integ := createTestIntegration(t, h.db, org.ID, "github-app")
	conn := createTestConnection(t, h.db, org.ID, integ.ID, "nango-conn-6")

	rr := h.doConnRequest(t, http.MethodPost, "/v1/connections/"+conn.ID.String()+"/token", nil, &org)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d; body: %s", rr.Code, rr.Body.String())
	}
}
