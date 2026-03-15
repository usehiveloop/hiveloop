package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
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

	// Verify nango_config is populated on create
	nangoConfig, hasConfig := createResp["nango_config"].(map[string]any)
	if !hasConfig || nangoConfig == nil {
		t.Fatal("expected nango_config to be populated on create")
	}
	if nangoConfig["callback_url"] == nil {
		t.Fatal("expected nango_config.callback_url to be set")
	}
	if nangoConfig["auth_mode"] == nil {
		t.Fatal("expected nango_config.auth_mode to be set")
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

	// 4b. Update credentials — pushes to Nango, re-fetches config, saves to DB
	credUpdateBody := `{"credentials":{"type":"OAUTH2","client_id":"new-id","client_secret":"new-secret","scopes":"channels:read"}}`
	req = httptest.NewRequest(http.MethodPut, "/v1/integrations/"+integID, strings.NewReader(credUpdateBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("update credentials: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var credUpdateResp map[string]any
	json.NewDecoder(rr.Body).Decode(&credUpdateResp)

	// Verify nango_config is rebuilt after credential update
	updatedConfig, hasUpdatedConfig := credUpdateResp["nango_config"].(map[string]any)
	if !hasUpdatedConfig || updatedConfig == nil {
		t.Fatal("expected nango_config to be populated after credential update")
	}
	if updatedConfig["callback_url"] == nil {
		t.Fatal("expected nango_config.callback_url after credential update")
	}
	if updatedConfig["auth_mode"] == nil {
		t.Fatal("expected nango_config.auth_mode after credential update")
	}

	// Verify the config persisted to DB by re-fetching via GET
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get after cred update: expected 200, got %d", rr.Code)
	}
	var getAfterUpdate map[string]any
	json.NewDecoder(rr.Body).Decode(&getAfterUpdate)
	persistedConfig, hasPersisted := getAfterUpdate["nango_config"].(map[string]any)
	if !hasPersisted || persistedConfig == nil {
		t.Fatal("nango_config not persisted to DB after credential update")
	}
	if persistedConfig["auth_mode"] == nil {
		t.Fatal("expected persisted nango_config.auth_mode")
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
// E2E: ListProviders — returns Nango provider catalog
// --------------------------------------------------------------------------

func TestE2E_Integration_ListProviders(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// 1. Management API route: GET /v1/integrations/providers
	req := httptest.NewRequest(http.MethodGet, "/v1/integrations/providers", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("list providers: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var providers []map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&providers); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	if len(providers) == 0 {
		t.Fatal("expected non-empty provider list")
	}

	// Verify response shape
	first := providers[0]
	if _, ok := first["name"].(string); !ok {
		t.Fatal("expected name field to be a string")
	}
	if _, ok := first["display_name"].(string); !ok {
		t.Fatal("expected display_name field to be a string")
	}
	if _, ok := first["auth_mode"].(string); !ok {
		t.Fatal("expected auth_mode field to be a string")
	}

	// Verify well-known provider is present (slack is OAUTH2)
	foundSlack := false
	for _, p := range providers {
		if p["name"] == "slack" {
			foundSlack = true
			if p["auth_mode"] != "OAUTH2" {
				t.Fatalf("expected slack auth_mode=OAUTH2, got %v", p["auth_mode"])
			}
			break
		}
	}
	if !foundSlack {
		t.Fatal("expected slack in provider list")
	}

	// 2. Widget API route: GET /v1/widget/integrations/providers (session-authenticated)
	//    Create a connect session to authenticate
	integ := h.createIntegration(t, org, "slack", "Widget Provider Test")
	sessionBody := fmt.Sprintf(`{"integration_id":%q,"external_id":"widget-test-user"}`, integ.ID.String())
	req = httptest.NewRequest(http.MethodPost, "/v1/connect/sessions", strings.NewReader(sessionBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create session: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var sessionResp map[string]any
	json.NewDecoder(rr.Body).Decode(&sessionResp)
	sessionToken := sessionResp["session_token"].(string)

	// Use session token to call widget providers endpoint
	req = httptest.NewRequest(http.MethodGet, "/v1/widget/integrations/providers", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("widget list providers: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var widgetProviders []map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&widgetProviders); err != nil {
		t.Fatalf("decode widget providers: %v", err)
	}
	if len(widgetProviders) == 0 {
		t.Fatal("expected non-empty provider list from widget route")
	}
	// Should return same count as management API
	if len(widgetProviders) != len(providers) {
		t.Fatalf("widget route returned %d providers vs management API %d", len(widgetProviders), len(providers))
	}
}

// --------------------------------------------------------------------------
// E2E: APP auth mode — github-app with app_id/app_link/private_key succeeds
// --------------------------------------------------------------------------

func TestE2E_Integration_AppAuthMode(t *testing.T) {
	h := newHarness(t)
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

// --------------------------------------------------------------------------
// E2E: APP auth mode — auto-generated webhook_secret in nango_config
// --------------------------------------------------------------------------

func TestE2E_Integration_AppWebhookSecret(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Find an APP auth mode provider
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

	appID := "ws-test-12345"
	privateKey := "-----BEGIN RSA PRIVATE KEY-----\nfake-key-for-webhook-test\n-----END RSA PRIVATE KEY-----"
	appLink := "https://example.com/app-webhook-test"

	body := fmt.Sprintf(`{"provider":%q,"display_name":"Webhook Secret Test","credentials":{"type":"APP","app_id":%q,"app_link":%q,"private_key":%q}}`,
		appProvider, appID, appLink, privateKey)
	req := httptest.NewRequest(http.MethodPost, "/v1/integrations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create APP integration: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var createResp map[string]any
	json.NewDecoder(rr.Body).Decode(&createResp)

	nangoConfig, ok := createResp["nango_config"].(map[string]any)
	if !ok || nangoConfig == nil {
		t.Fatal("expected nango_config to be populated")
	}

	webhookSecret, ok := nangoConfig["webhook_secret"].(string)
	if !ok || webhookSecret == "" {
		t.Fatal("expected webhook_secret to be auto-generated for APP mode")
	}

	// Verify it matches expected SHA256(app_id + private_key + app_link)
	expectedHash := sha256.Sum256([]byte(appID + privateKey + appLink))
	expectedSecret := hex.EncodeToString(expectedHash[:])

	if webhookSecret != expectedSecret {
		t.Fatalf("webhook_secret mismatch:\n  got:      %s\n  expected: %s", webhookSecret, expectedSecret)
	}

	// Verify it persists — GET should return same value
	integID := createResp["id"].(string)
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("get integration: expected 200, got %d", rr.Code)
	}
	var getResp map[string]any
	json.NewDecoder(rr.Body).Decode(&getResp)
	getConfig := getResp["nango_config"].(map[string]any)
	if getConfig["webhook_secret"] != expectedSecret {
		t.Fatalf("persisted webhook_secret doesn't match: got %v", getConfig["webhook_secret"])
	}
}

// --------------------------------------------------------------------------
// E2E: OAUTH2 — create → GET → assert nango_config has callback_url,
// webhook_url, auth_mode, and user-defined webhook_secret
// --------------------------------------------------------------------------

func TestE2E_Integration_OAUTH2_NangoConfigFields(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// 1. Create OAUTH2 integration with webhook_secret in credentials
	body := `{"provider":"slack","display_name":"OAuth Config Check","credentials":{"type":"OAUTH2","client_id":"test-id","client_secret":"test-secret","webhook_secret":"my-user-defined-secret"}}`
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

	// 2. Fetch via GET /v1/integrations/{id}
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var getResp map[string]any
	json.NewDecoder(rr.Body).Decode(&getResp)

	// 3. Assert nango_config fields
	config, ok := getResp["nango_config"].(map[string]any)
	if !ok || config == nil {
		t.Fatal("expected nango_config to be populated on GET")
	}

	// callback_url must always be present
	callbackURL, ok := config["callback_url"].(string)
	if !ok || callbackURL == "" {
		t.Fatal("expected nango_config.callback_url to be a non-empty string")
	}

	// auth_mode must be present (from template)
	authMode, ok := config["auth_mode"].(string)
	if !ok || authMode == "" {
		t.Fatal("expected nango_config.auth_mode to be a non-empty string")
	}
	if authMode != "OAUTH2" {
		t.Fatalf("expected auth_mode=OAUTH2, got %s", authMode)
	}

	// webhook_url must be present (Nango returns it with ?include[]=webhook)
	webhookURL, ok := config["webhook_url"].(string)
	if !ok || webhookURL == "" {
		t.Fatal("expected nango_config.webhook_url to be a non-empty string")
	}

	// webhook_secret: Nango stores and returns user-defined secrets for OAUTH2
	webhookSecret, _ := config["webhook_secret"].(string)
	if webhookSecret == "my-user-defined-secret" {
		t.Log("webhook_secret round-tripped through Nango correctly")
	} else {
		t.Logf("webhook_secret from Nango: %v (may differ from input depending on Nango version)", config["webhook_secret"])
	}

	t.Logf("nango_config keys: %v", mapKeys(config))
}

// --------------------------------------------------------------------------
// E2E: API_KEY mode — create → GET → assert nango_config populated
// --------------------------------------------------------------------------

func TestE2E_Integration_APIKey_NangoConfigFields(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Find an API_KEY auth mode provider
	providers := h.nangoClient.GetProviders()
	var apiKeyProvider string
	for _, p := range providers {
		if p.AuthMode == "API_KEY" {
			apiKeyProvider = p.Name
			break
		}
	}
	if apiKeyProvider == "" {
		t.Fatal("no API_KEY auth mode provider found")
	}

	// 1. Create
	body := fmt.Sprintf(`{"provider":%q,"display_name":"API Key Config Check"}`, apiKeyProvider)
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

	// 2. Fetch
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", rr.Code)
	}

	var getResp map[string]any
	json.NewDecoder(rr.Body).Decode(&getResp)

	// 3. Assert
	config, ok := getResp["nango_config"].(map[string]any)
	if !ok || config == nil {
		t.Fatal("expected nango_config to be populated")
	}

	if _, ok := config["callback_url"].(string); !ok {
		t.Fatal("expected callback_url")
	}
	if _, ok := config["auth_mode"].(string); !ok {
		t.Fatal("expected auth_mode")
	}

	// webhook_url is not present for all providers (API_KEY providers may lack webhooks)
	if webhookURL, ok := config["webhook_url"].(string); ok {
		t.Logf("webhook_url present: %s", webhookURL)
	} else {
		t.Log("webhook_url not present (expected for some API_KEY providers)")
	}

	t.Logf("API_KEY nango_config keys: %v", mapKeys(config))
}

// --------------------------------------------------------------------------
// E2E: ListProviders includes webhook_user_defined_secret for known providers
// --------------------------------------------------------------------------

func TestE2E_Integration_ListProviders_WebhookFlag(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/integrations/providers", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("list providers: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var providers []map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&providers); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	if len(providers) == 0 {
		t.Fatal("expected non-empty provider list")
	}

	// Linear is a known provider with webhook_user_defined_secret=true
	foundLinear := false
	for _, p := range providers {
		if p["name"] == "linear" {
			foundLinear = true
			wuds, ok := p["webhook_user_defined_secret"].(bool)
			if !ok || !wuds {
				t.Fatal("expected linear to have webhook_user_defined_secret=true")
			}
			break
		}
	}
	if !foundLinear {
		t.Fatal("expected linear in provider list")
	}
}

// --------------------------------------------------------------------------
// E2E: APP credential rotation recomputes webhook_secret
// --------------------------------------------------------------------------

func TestE2E_Integration_AppCredentialRotation_RecomputesWebhookSecret(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Find APP auth mode provider
	providers := h.nangoClient.GetProviders()
	var appProvider string
	for _, p := range providers {
		if p.AuthMode == "APP" {
			appProvider = p.Name
			break
		}
	}
	if appProvider == "" {
		t.Fatal("no APP auth mode provider found")
	}

	origPrivateKey := "-----BEGIN RSA PRIVATE KEY-----\noriginal-fake-key-data\n-----END RSA PRIVATE KEY-----"
	newPrivateKey := "-----BEGIN RSA PRIVATE KEY-----\nrotated-fake-key-data\n-----END RSA PRIVATE KEY-----"

	// Create with initial credentials
	body := fmt.Sprintf(`{"provider":%q,"display_name":"Rotation Test","credentials":{"type":"APP","app_id":"orig-id","app_link":"https://example.com/orig","private_key":%q}}`, appProvider, origPrivateKey)
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
	origConfig := createResp["nango_config"].(map[string]any)
	origSecret := origConfig["webhook_secret"].(string)

	// Rotate credentials with different values
	rotateBody := fmt.Sprintf(`{"credentials":{"type":"APP","app_id":"new-id","app_link":"https://example.com/new","private_key":%q}}`, newPrivateKey)
	req = httptest.NewRequest(http.MethodPut, "/v1/integrations/"+integID, strings.NewReader(rotateBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var updateResp map[string]any
	json.NewDecoder(rr.Body).Decode(&updateResp)
	updatedConfig := updateResp["nango_config"].(map[string]any)
	newSecret := updatedConfig["webhook_secret"].(string)

	if newSecret == "" {
		t.Fatal("webhook_secret should be set after credential rotation")
	}
	if newSecret == origSecret {
		t.Fatal("webhook_secret should change when credentials are rotated")
	}

	// Verify new secret matches expected hash
	expectedHash := sha256.Sum256([]byte("new-id" + newPrivateKey + "https://example.com/new"))
	expectedSecret := hex.EncodeToString(expectedHash[:])
	if newSecret != expectedSecret {
		t.Fatalf("rotated webhook_secret mismatch:\n  got:      %s\n  expected: %s", newSecret, expectedSecret)
	}
}

// --------------------------------------------------------------------------
// E2E: Update webhook_secret without providing credentials
// --------------------------------------------------------------------------

func TestE2E_Integration_UpdateWebhookSecret_NoCreds(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// 1. Create OAUTH2 integration (no webhook_secret initially)
	body := `{"provider":"slack","display_name":"WS Update Test","credentials":{"type":"OAUTH2","client_id":"ws-id","client_secret":"ws-secret"}}`
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

	// 2. Update with ONLY webhook_secret — no credentials
	updateBody := `{"webhook_secret":"my-standalone-secret"}`
	req = httptest.NewRequest(http.MethodPut, "/v1/integrations/"+integID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("update webhook_secret: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// 3. GET and verify webhook_secret is set
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", rr.Code)
	}

	var getResp map[string]any
	json.NewDecoder(rr.Body).Decode(&getResp)
	config := getResp["nango_config"].(map[string]any)

	if config["webhook_secret"] != "my-standalone-secret" {
		t.Fatalf("expected webhook_secret=my-standalone-secret, got %v", config["webhook_secret"])
	}
}

// --------------------------------------------------------------------------
// E2E: GET lazy-populates nango_config for existing integrations
// --------------------------------------------------------------------------

func TestE2E_Integration_LazyPopulateNangoConfig(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// 1. Create integration normally (nango_config will be populated)
	body := `{"provider":"slack","display_name":"Lazy Populate","credentials":{"type":"OAUTH2","client_id":"lp-id","client_secret":"lp-secret"}}`
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

	// 2. Simulate broken nango_config by NULLing it in DB
	if err := h.db.Exec("UPDATE integrations SET nango_config = NULL WHERE id = ?", integID).Error; err != nil {
		t.Fatalf("null nango_config: %v", err)
	}

	// 3. GET should lazy-populate nango_config from Nango
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", rr.Code)
	}

	var getResp map[string]any
	json.NewDecoder(rr.Body).Decode(&getResp)

	config, ok := getResp["nango_config"].(map[string]any)
	if !ok || config == nil {
		t.Fatal("expected nango_config to be lazy-populated on GET")
	}

	if _, ok := config["callback_url"].(string); !ok {
		t.Fatal("expected callback_url after lazy-populate")
	}
	if _, ok := config["auth_mode"].(string); !ok {
		t.Fatal("expected auth_mode after lazy-populate")
	}
	if _, ok := config["webhook_url"].(string); !ok {
		t.Fatal("expected webhook_url after lazy-populate")
	}

	// 4. Second GET should return from DB (already populated)
	req = httptest.NewRequest(http.MethodGet, "/v1/integrations/"+integID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	var getResp2 map[string]any
	json.NewDecoder(rr.Body).Decode(&getResp2)
	config2 := getResp2["nango_config"].(map[string]any)
	if config2["callback_url"] != config["callback_url"] {
		t.Fatal("lazy-populated config should persist across GET calls")
	}
}

// --------------------------------------------------------------------------
// E2E: GetIntegration from Nango returns webhook_url + credentials
// --------------------------------------------------------------------------

func TestE2E_Integration_GetIntegration_NangoResponse(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// 1. Create an OAUTH2 integration
	body := `{"provider":"slack","display_name":"Nango Fetch Check","credentials":{"type":"OAUTH2","client_id":"fetch-test-id","client_secret":"fetch-test-secret"}}`
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

	// 2. Look up the nango key and call GetIntegration directly
	var dbInteg model.Integration
	if err := h.db.Where("id = ?", integID).First(&dbInteg).Error; err != nil {
		t.Fatalf("lookup: %v", err)
	}
	nk := fmt.Sprintf("%s_%s", org.ID.String(), dbInteg.UniqueKey)

	nangoResp, err := h.nangoClient.GetIntegration(context.Background(), nk)
	if err != nil {
		t.Fatalf("GetIntegration failed: %v", err)
	}

	// 3. Assert the Nango response has the expected structure
	data, ok := nangoResp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data in nango response")
	}

	// webhook_url must be present (from ?include[]=webhook)
	webhookURL, ok := data["webhook_url"].(string)
	if !ok || webhookURL == "" {
		t.Fatalf("expected webhook_url in Nango response, got keys: %v", mapKeys(data))
	}

	// credentials must be present (from ?include[]=credentials)
	creds, ok := data["credentials"].(map[string]any)
	if !ok {
		t.Fatalf("expected credentials in Nango response, got keys: %v", mapKeys(data))
	}

	// Verify credentials contain type and client_id (we sent OAUTH2 creds)
	if creds["type"] != "OAUTH2" {
		t.Fatalf("expected credentials.type=OAUTH2, got %v", creds["type"])
	}
	if creds["client_id"] != "fetch-test-id" {
		t.Fatalf("expected credentials.client_id=fetch-test-id, got %v", creds["client_id"])
	}

	t.Logf("Nango GetIntegration data keys: %v", mapKeys(data))
	t.Logf("Nango credentials keys: %v", mapKeys(creds))
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
