package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/useportal/llmvault/internal/middleware"
	"github.com/useportal/llmvault/internal/model"
)

// --------------------------------------------------------------------------
// E2E: Integration CRUD lifecycle (OAUTH2 provider with credentials)
// --------------------------------------------------------------------------

func TestE2E_Integration_CRUD(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// 1. Create integration with OAUTH2 credentials
	body := `{"provider":"slack","display_name":"Slack Production","credentials":{"type":"OAUTH2","client_id":"test-id","client_secret":"test-secret","scopes":"channels:read,chat:write"},"meta":{"team":"engineering"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create integration: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var createResp map[string]any
	json.NewDecoder(rr.Body).Decode(&createResp)
	integID := createResp["id"].(string)

	if createResp["provider"] != "slack" {
		t.Fatalf("expected provider=slack, got %v", createResp["provider"])
	}
	if createResp["display_name"] != "Slack Production" {
		t.Fatalf("expected display_name=Slack Production, got %v", createResp["display_name"])
	}
	meta := createResp["meta"].(map[string]any)
	if meta["team"] != "engineering" {
		t.Fatalf("expected meta.team=engineering, got %v", meta["team"])
	}

	// 2. Get integration
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("get integration: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var getResp map[string]any
	json.NewDecoder(rr.Body).Decode(&getResp)
	if getResp["id"] != integID {
		t.Fatalf("get returned wrong id: %v", getResp["id"])
	}

	// 3. List integrations
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations", nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("list integrations: expected 200, got %d", rr.Code)
	}
	listResp := decodePaginatedList(t, rr)
	found := false
	for _, integ := range listResp {
		if integ["id"] == integID {
			found = true
		}
	}
	if !found {
		t.Fatal("created integration not in list")
	}

	// 4. Update integration — change display name and meta
	updateBody := `{"display_name":"Slack Production v2","meta":{"team":"platform"}}`
	req = httptest.NewRequest(http.MethodPut, "/v1/integrations/"+integID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("update integration: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var updateResp map[string]any
	json.NewDecoder(rr.Body).Decode(&updateResp)
	if updateResp["display_name"] != "Slack Production v2" {
		t.Fatalf("expected updated display_name=Slack Production v2, got %v", updateResp["display_name"])
	}
	updatedMeta := updateResp["meta"].(map[string]any)
	if updatedMeta["team"] != "platform" {
		t.Fatalf("expected updated meta.team=platform, got %v", updatedMeta["team"])
	}

	// 5. Delete integration
	req = httptest.NewRequest(http.MethodDelete, "/v1/integrations/"+integID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("delete integration: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify it's gone (soft-deleted)
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Multiple integrations per provider
// --------------------------------------------------------------------------

func TestE2E_Integration_MultiplePerProvider(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create 3 Slack integrations with different display names
	for i := 0; i < 3; i++ {
		body := fmt.Sprintf(`{"provider":"slack","display_name":"Slack %d","credentials":{"type":"OAUTH2","client_id":"id-%d","client_secret":"secret-%d"}}`, i, i, i)
		req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		rr := httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create integration %d: expected 201, got %d: %s", i, rr.Code, rr.Body.String())
		}
	}

	// List and verify all 3 exist
	req := httptest.NewRequest(http.MethodGet, "/v1/integrations?provider=slack", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}
	list := decodePaginatedList(t, rr)
	if len(list) != 3 {
		t.Fatalf("expected 3 slack integrations, got %d", len(list))
	}
}

// --------------------------------------------------------------------------
// E2E: Tenant isolation — org2 can't see/update/delete org1's integrations
// --------------------------------------------------------------------------

func TestE2E_Integration_TenantIsolation(t *testing.T) {
	h := newHarness(t)
	org1 := h.createOrg(t)
	org2 := h.createOrg(t)

	// Create integration in org1
	body := `{"provider":"slack","display_name":"Isolated","credentials":{"type":"OAUTH2","client_id":"id","client_secret":"secret"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org1)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", rr.Code)
	}
	var createResp map[string]any
	json.NewDecoder(rr.Body).Decode(&createResp)
	integID := createResp["id"].(string)

	// org2 should NOT see it via GET
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integID, nil)
	req = middleware.WithOrg(req, &org2)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("org2 GET: expected 404, got %d", rr.Code)
	}

	// org2 should NOT see it via list
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations", nil)
	req = middleware.WithOrg(req, &org2)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("org2 list: expected 200, got %d", rr.Code)
	}
	list := decodePaginatedList(t, rr)
	for _, integ := range list {
		if integ["id"] == integID {
			t.Fatal("org2 can see org1's integration — tenant isolation violated")
		}
	}

	// org2 should NOT be able to update it
	updateBody := `{"display_name":"Hacked"}`
	req = httptest.NewRequest(http.MethodPut, "/v1/integrations/"+integID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org2)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("org2 update: expected 404, got %d", rr.Code)
	}

	// org2 should NOT be able to delete it
	req = httptest.NewRequest(http.MethodDelete, "/v1/integrations/"+integID, nil)
	req = middleware.WithOrg(req, &org2)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("org2 delete: expected 404, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Metadata filtering
// --------------------------------------------------------------------------

func TestE2E_Integration_MetadataFiltering(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create integrations with different metadata
	for i, data := range []struct {
		meta string
	}{
		{`{"env":"production","team":"backend"}`},
		{`{"env":"staging","team":"frontend"}`},
		{`{"env":"production","team":"frontend"}`},
	} {
		body := fmt.Sprintf(`{"provider":"slack","display_name":"Meta Test %d","credentials":{"type":"OAUTH2","client_id":"id","client_secret":"secret"},"meta":%s}`, i, data.meta)
		req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		rr := httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create %d: expected 201, got %d: %s", i, rr.Code, rr.Body.String())
		}
	}

	// Filter by env=production — should return 2
	req := httptest.NewRequest(http.MethodGet, `/v1/integrations?meta={"env":"production"}`, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("filter: expected 200, got %d", rr.Code)
	}
	filtered := decodePaginatedList(t, rr)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 integrations with env=production, got %d", len(filtered))
	}

	// Filter by env=production AND team=frontend — should return 1
	req = httptest.NewRequest(http.MethodGet, `/v1/integrations?meta={"env":"production","team":"frontend"}`, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("multi-filter: expected 200, got %d", rr.Code)
	}
	filtered = decodePaginatedList(t, rr)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 integration with env=production,team=frontend, got %d", len(filtered))
	}
}

// --------------------------------------------------------------------------
// E2E: Pagination
// --------------------------------------------------------------------------

func TestE2E_Integration_Pagination(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create 5 integrations
	for i := 0; i < 5; i++ {
		body := fmt.Sprintf(`{"provider":"slack","display_name":"Page %d","credentials":{"type":"OAUTH2","client_id":"id-%d","client_secret":"secret-%d"}}`, i, i, i)
		req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		rr := httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create %d: expected 201, got %d: %s", i, rr.Code, rr.Body.String())
		}
	}

	// Request first page of 2
	req := httptest.NewRequest(http.MethodGet, "/v1/integrations?limit=2", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("page 1: expected 200, got %d", rr.Code)
	}
	var page1 struct {
		Data       []map[string]any `json:"data"`
		HasMore    bool             `json:"has_more"`
		NextCursor *string          `json:"next_cursor"`
	}
	json.NewDecoder(rr.Body).Decode(&page1)

	if len(page1.Data) != 2 {
		t.Fatalf("page 1: expected 2 items, got %d", len(page1.Data))
	}
	if !page1.HasMore {
		t.Fatal("page 1: expected has_more=true")
	}
	if page1.NextCursor == nil {
		t.Fatal("page 1: expected next_cursor")
	}

	// Request second page using cursor
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations?limit=2&cursor="+*page1.NextCursor, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("page 2: expected 200, got %d", rr.Code)
	}
	var page2 struct {
		Data       []map[string]any `json:"data"`
		HasMore    bool             `json:"has_more"`
		NextCursor *string          `json:"next_cursor"`
	}
	json.NewDecoder(rr.Body).Decode(&page2)

	if len(page2.Data) != 2 {
		t.Fatalf("page 2: expected 2 items, got %d", len(page2.Data))
	}
	if !page2.HasMore {
		t.Fatal("page 2: expected has_more=true")
	}

	// Request third page — should have 1 item
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations?limit=2&cursor="+*page2.NextCursor, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("page 3: expected 200, got %d", rr.Code)
	}
	var page3 struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	json.NewDecoder(rr.Body).Decode(&page3)

	if len(page3.Data) != 1 {
		t.Fatalf("page 3: expected 1 item, got %d", len(page3.Data))
	}
	if page3.HasMore {
		t.Fatal("page 3: expected has_more=false")
	}
}

// --------------------------------------------------------------------------
// E2E: Nango sync verification — integration exists in Nango after create,
// gone after delete
// --------------------------------------------------------------------------

func TestE2E_Integration_NangoSync(t *testing.T) {
	h := newHarness(t)
	if h.nangoClient == nil {
		t.Fatal("NANGO_ENDPOINT must be set")
	}
	org := h.createOrg(t)

	// Create integration
	body := `{"provider":"slack","display_name":"Nango Sync","credentials":{"type":"OAUTH2","client_id":"test-id","client_secret":"test-secret"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var createResp map[string]any
	json.NewDecoder(rr.Body).Decode(&createResp)
	integID := createResp["id"].(string)

	// Look up the auto-generated unique_key from the DB to verify Nango sync
	var dbInteg model.Integration
	if err := h.db.Where("id = ?", integID).First(&dbInteg).Error; err != nil {
		t.Fatalf("lookup integration in DB: %v", err)
	}

	// Verify integration exists in Nango using the auto-generated key
	nk := fmt.Sprintf("%s_%s", org.ID.String(), dbInteg.UniqueKey)
	nangoResp, err := h.nangoClient.GetIntegration(context.Background(), nk)
	if err != nil {
		t.Fatalf("get from Nango: %v", err)
	}
	if nangoResp == nil {
		t.Fatal("integration not found in Nango after create")
	}

	// Delete integration
	req = httptest.NewRequest(http.MethodDelete, "/v1/integrations/"+integID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify integration is gone from Nango
	_, err = h.nangoClient.GetIntegration(context.Background(), nk)
	if err == nil {
		t.Fatal("integration should be gone from Nango after delete")
	}

	// Verify soft-deleted in DB
	var integ model.Integration
	result := h.db.Unscoped().Where("id = ?", integID).First(&integ)
	if result.Error != nil {
		t.Fatalf("should still exist in DB (soft-deleted): %v", result.Error)
	}
	if integ.DeletedAt == nil {
		t.Fatal("expected deleted_at to be set")
	}
}

// --------------------------------------------------------------------------
// E2E: Invalid provider returns 400
// --------------------------------------------------------------------------

func TestE2E_Integration_InvalidProvider(t *testing.T) {
	h := newHarness(t)
	if h.nangoClient == nil {
		t.Fatal("NANGO_ENDPOINT must be set")
	}
	org := h.createOrg(t)

	body := `{"provider":"nonexistent-xyz-12345","display_name":"Bad Provider"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid provider: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "nonexistent-xyz-12345") {
		t.Fatalf("error should mention provider name, got: %s", resp["error"])
	}
}

// --------------------------------------------------------------------------
// E2E: Credential validation — OAUTH2 without required fields returns 400
// --------------------------------------------------------------------------

func TestE2E_Integration_CredentialValidation(t *testing.T) {
	h := newHarness(t)
	if h.nangoClient == nil {
		t.Fatal("NANGO_ENDPOINT must be set")
	}
	org := h.createOrg(t)

	// OAUTH2 provider without client_id — should fail
	body := `{"provider":"slack","display_name":"Bad Creds","credentials":{"type":"OAUTH2","client_secret":"secret"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing client_id: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "client_id") {
		t.Fatalf("error should mention client_id, got: %s", resp["error"])
	}
}

// --------------------------------------------------------------------------
// E2E: No-credential auth modes — API_KEY provider without credentials succeeds
// --------------------------------------------------------------------------

func TestE2E_Integration_NoCredentialAuthMode(t *testing.T) {
	h := newHarness(t)
	if h.nangoClient == nil {
		t.Fatal("NANGO_ENDPOINT must be set")
	}
	org := h.createOrg(t)

	// Find an API_KEY auth mode provider from the cache
	providers := h.nangoClient.GetProviders()
	var apiKeyProvider string
	for _, p := range providers {
		if p.AuthMode == "API_KEY" {
			apiKeyProvider = p.Name
			break
		}
	}
	if apiKeyProvider == "" {
		t.Fatal("no API_KEY auth mode provider found in Nango catalog")
	}

	// Create integration without credentials — should succeed
	body := fmt.Sprintf(`{"provider":%q,"display_name":"API Key Test"}`, apiKeyProvider)
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("API_KEY provider without credentials: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Providing credentials for API_KEY provider should fail
	bodyWithCreds := fmt.Sprintf(`{"provider":%q,"display_name":"API Key Bad","credentials":{"type":"API_KEY","client_id":"x"}}`, apiKeyProvider)
	req = httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(bodyWithCreds))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("API_KEY provider with credentials: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: APP auth mode — github-app with app_id/app_link/private_key succeeds
// --------------------------------------------------------------------------

func TestE2E_Integration_AppAuthMode(t *testing.T) {
	h := newHarness(t)
	if h.nangoClient == nil {
		t.Fatal("NANGO_ENDPOINT must be set")
	}
	org := h.createOrg(t)

	// Find an APP auth mode provider from the cache
	providers := h.nangoClient.GetProviders()
	var appProvider string
	for _, p := range providers {
		if p.AuthMode == "APP" {
			appProvider = p.Name
			break
		}
	}
	if appProvider == "" {
		t.Fatal("no APP auth mode provider found in Nango catalog")
	}

	// Create with APP credentials
	body := fmt.Sprintf(`{"provider":%q,"display_name":"App Test","credentials":{"type":"APP","app_id":"12345","app_link":"https://example.com/app","private_key":"-----BEGIN RSA PRIVATE KEY-----\nfake\n-----END RSA PRIVATE KEY-----"}}`, appProvider)
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("APP provider: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Missing required APP fields should fail
	bodyMissing := fmt.Sprintf(`{"provider":%q,"display_name":"App Bad","credentials":{"type":"APP","app_id":"12345"}}`, appProvider)
	req = httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(bodyMissing))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("APP provider missing fields: expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
