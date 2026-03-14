package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/token"
)

// --------------------------------------------------------------------------
// E2E: Identity CRUD lifecycle (Postgres + real infra)
// --------------------------------------------------------------------------

func TestE2E_Identity_CRUD(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// 1. Create identity
	body := `{"external_id":"customer_42","meta":{"plan":"pro","company":"Acme Corp"},"ratelimits":[{"name":"requests","limit":100,"duration":60000}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create identity: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var createResp map[string]any
	json.NewDecoder(rr.Body).Decode(&createResp)
	identID := createResp["id"].(string)

	if createResp["external_id"] != "customer_42" {
		t.Fatalf("expected external_id=customer_42, got %v", createResp["external_id"])
	}
	meta := createResp["meta"].(map[string]any)
	if meta["plan"] != "pro" {
		t.Fatalf("expected meta.plan=pro, got %v", meta["plan"])
	}
	rls := createResp["ratelimits"].([]any)
	if len(rls) != 1 {
		t.Fatalf("expected 1 ratelimit, got %d", len(rls))
	}
	rl := rls[0].(map[string]any)
	if rl["name"] != "requests" {
		t.Fatalf("expected ratelimit name=requests, got %v", rl["name"])
	}

	// 2. Get identity
	req = httptest.NewRequest(http.MethodGet, "/v1/identities/"+identID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("get identity: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var getResp map[string]any
	json.NewDecoder(rr.Body).Decode(&getResp)
	if getResp["id"] != identID {
		t.Fatalf("get returned wrong id: %v", getResp["id"])
	}

	// 3. List identities
	req = httptest.NewRequest(http.MethodGet, "/v1/identities", nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("list identities: expected 200, got %d", rr.Code)
	}
	listResp := decodePaginatedList(t, rr)
	found := false
	for _, ident := range listResp {
		if ident["id"] == identID {
			found = true
		}
	}
	if !found {
		t.Fatal("created identity not in list")
	}

	// 4. Update identity — change meta and rate limits
	updateBody := `{"meta":{"plan":"enterprise","company":"Acme Corp"},"ratelimits":[{"name":"requests","limit":500,"duration":60000},{"name":"tokens","limit":10000,"duration":3600000}]}`
	req = httptest.NewRequest(http.MethodPut, "/v1/identities/"+identID, strings.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("update identity: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var updateResp map[string]any
	json.NewDecoder(rr.Body).Decode(&updateResp)
	updatedMeta := updateResp["meta"].(map[string]any)
	if updatedMeta["plan"] != "enterprise" {
		t.Fatalf("expected updated meta.plan=enterprise, got %v", updatedMeta["plan"])
	}
	updatedRLs := updateResp["ratelimits"].([]any)
	if len(updatedRLs) != 2 {
		t.Fatalf("expected 2 ratelimits after update, got %d", len(updatedRLs))
	}

	// 5. Delete identity
	req = httptest.NewRequest(http.MethodDelete, "/v1/identities/"+identID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("delete identity: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify it's gone
	req = httptest.NewRequest(http.MethodGet, "/v1/identities/"+identID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Duplicate external_id returns 409
// --------------------------------------------------------------------------

func TestE2E_Identity_DuplicateExternalID(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	body := `{"external_id":"dup_test_42"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", rr.Code)
	}

	// Second create with same external_id
	req = httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("duplicate create: expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// E2E: Credential linked to identity via identity_id
// --------------------------------------------------------------------------

func TestE2E_Identity_LinkCredentialByIdentityID(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create identity
	identBody := `{"external_id":"link_test_id"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(identBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create identity: expected 201, got %d", rr.Code)
	}
	var identResp map[string]any
	json.NewDecoder(rr.Body).Decode(&identResp)
	identID := identResp["id"].(string)

	// Create credential with identity_id
	credBody := fmt.Sprintf(`{"label":"linked","base_url":"https://api.example.com","auth_scheme":"bearer","api_key":"sk-test","identity_id":%q}`, identID)
	req = httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(credBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create credential: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var credResp map[string]any
	json.NewDecoder(rr.Body).Decode(&credResp)
	if credResp["identity_id"] != identID {
		t.Fatalf("expected identity_id=%s, got %v", identID, credResp["identity_id"])
	}

	// List credentials filtered by identity_id
	req = httptest.NewRequest(http.MethodGet, "/v1/credentials?identity_id="+identID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list by identity_id: expected 200, got %d", rr.Code)
	}
	creds := decodePaginatedList(t, rr)
	if len(creds) == 0 {
		t.Fatal("expected at least 1 credential filtered by identity_id")
	}
	for _, c := range creds {
		if c["identity_id"] != identID {
			t.Fatalf("filtered credential has wrong identity_id: %v", c["identity_id"])
		}
	}
}

// --------------------------------------------------------------------------
// E2E: Auto-upsert identity via external_id on credential creation
// --------------------------------------------------------------------------

func TestE2E_Identity_AutoUpsertViaExternalID(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	extID := fmt.Sprintf("auto_upsert_%s", uuid.New().String()[:8])

	// Create credential with external_id — should auto-create identity
	credBody := fmt.Sprintf(`{"label":"auto","base_url":"https://api.example.com","auth_scheme":"bearer","api_key":"sk-test","external_id":%q}`, extID)
	req := httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(credBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create credential: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var credResp map[string]any
	json.NewDecoder(rr.Body).Decode(&credResp)
	identID := credResp["identity_id"]
	if identID == nil || identID == "" {
		t.Fatal("expected identity_id to be set after auto-upsert")
	}

	// Verify identity was created in DB
	req = httptest.NewRequest(http.MethodGet, "/v1/identities?external_id="+extID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list identities: expected 200, got %d", rr.Code)
	}
	identities := decodePaginatedList(t, rr)
	if len(identities) != 1 {
		t.Fatalf("expected 1 auto-created identity, got %d", len(identities))
	}
	if identities[0]["external_id"] != extID {
		t.Fatalf("auto-created identity has wrong external_id: %v", identities[0]["external_id"])
	}

	// Create second credential with same external_id — should reuse identity
	credBody2 := fmt.Sprintf(`{"label":"auto2","base_url":"https://api.example.com","auth_scheme":"bearer","api_key":"sk-test2","external_id":%q}`, extID)
	req = httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(credBody2))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create credential 2: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var credResp2 map[string]any
	json.NewDecoder(rr.Body).Decode(&credResp2)
	if credResp2["identity_id"] != identID {
		t.Fatalf("second credential should reuse identity: expected %v, got %v", identID, credResp2["identity_id"])
	}

	// List by external_id should return both credentials
	req = httptest.NewRequest(http.MethodGet, "/v1/credentials?external_id="+extID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list by external_id: expected 200, got %d", rr.Code)
	}
	filteredCreds := decodePaginatedList(t, rr)
	if len(filteredCreds) != 2 {
		t.Fatalf("expected 2 credentials for external_id=%s, got %d", extID, len(filteredCreds))
	}
}

// --------------------------------------------------------------------------
// E2E: Metadata filtering with JSONB @> containment
// --------------------------------------------------------------------------

func TestE2E_Identity_MetadataFiltering(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create identities with different metadata
	for _, data := range []struct {
		extID string
		meta  string
	}{
		{"meta_filter_1", `{"plan":"pro","region":"us"}`},
		{"meta_filter_2", `{"plan":"enterprise","region":"eu"}`},
		{"meta_filter_3", `{"plan":"pro","region":"eu"}`},
	} {
		body := fmt.Sprintf(`{"external_id":%q,"meta":%s}`, data.extID, data.meta)
		req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		rr := httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create identity %s: expected 201, got %d: %s", data.extID, rr.Code, rr.Body.String())
		}
	}

	// Filter by plan=pro — should return 2
	req := httptest.NewRequest(http.MethodGet, `/v1/identities?meta={"plan":"pro"}`, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("filter by meta: expected 200, got %d", rr.Code)
	}
	filtered := decodePaginatedList(t, rr)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 identities with plan=pro, got %d", len(filtered))
	}

	// Filter by plan=enterprise AND region=eu — should return 1
	req = httptest.NewRequest(http.MethodGet, `/v1/identities?meta={"plan":"enterprise","region":"eu"}`, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("filter by multi-meta: expected 200, got %d", rr.Code)
	}
	filtered = decodePaginatedList(t, rr)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 identity with plan=enterprise,region=eu, got %d", len(filtered))
	}
}

// --------------------------------------------------------------------------
// E2E: Credential metadata filtering
// --------------------------------------------------------------------------

func TestE2E_Credential_MetadataFiltering(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create credentials with different metadata
	for _, data := range []struct {
		label string
		meta  string
	}{
		{"prod-openai", `,"meta":{"env":"production","provider":"openai"}`},
		{"staging-openai", `,"meta":{"env":"staging","provider":"openai"}`},
		{"prod-anthropic", `,"meta":{"env":"production","provider":"anthropic"}`},
	} {
		body := fmt.Sprintf(`{"label":%q,"base_url":"https://api.example.com","auth_scheme":"bearer","api_key":"sk-test"%s}`, data.label, data.meta)
		req := httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		rr := httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create credential %s: expected 201, got %d: %s", data.label, rr.Code, rr.Body.String())
		}
	}

	// Filter by env=production — should return 2
	req := httptest.NewRequest(http.MethodGet, `/v1/credentials?meta={"env":"production"}`, nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("filter credentials by meta: expected 200, got %d", rr.Code)
	}
	filtered := decodePaginatedList(t, rr)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 credentials with env=production, got %d", len(filtered))
	}

	// Filter by provider=anthropic — should return 1
	req = httptest.NewRequest(http.MethodGet, `/v1/credentials?meta={"provider":"anthropic"}`, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("filter by provider: expected 200, got %d", rr.Code)
	}
	filtered = decodePaginatedList(t, rr)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 credential with provider=anthropic, got %d", len(filtered))
	}
}

// --------------------------------------------------------------------------
// E2E: Identity-level shared rate limiting across 2 credentials
// --------------------------------------------------------------------------

func TestE2E_Identity_SharedRateLimit(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Clean Redis rate limit keys on test end
	t.Cleanup(func() {
		h.redisClient.FlushDB(context.Background())
	})

	// Create identity with very low rate limit: 3 requests per 60 seconds
	identBody := `{"external_id":"ratelimit_test","ratelimits":[{"name":"requests","limit":3,"duration":60000}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(identBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create identity: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var identResp map[string]any
	json.NewDecoder(rr.Body).Decode(&identResp)
	identID := identResp["id"].(string)

	// Create echo server
	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer echoServer.Close()

	// Create 2 credentials linked to the same identity
	var credIDs []string
	var tokens []string
	for i := 0; i < 2; i++ {
		credBody := fmt.Sprintf(`{"label":"rl-cred-%d","base_url":%q,"auth_scheme":"bearer","api_key":"sk-test-%d","identity_id":%q}`,
			i, echoServer.URL, i, identID)
		req = httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(credBody))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		rr = httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create credential %d: expected 201, got %d: %s", i, rr.Code, rr.Body.String())
		}
		var credResp map[string]any
		json.NewDecoder(rr.Body).Decode(&credResp)
		credIDs = append(credIDs, credResp["id"].(string))

		// Mint token for each credential
		tokBody := fmt.Sprintf(`{"credential_id":%q,"ttl":"1h"}`, credResp["id"])
		req = httptest.NewRequest(http.MethodPost, "/v1/tokens", strings.NewReader(tokBody))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		rr = httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("mint token %d: expected 201, got %d: %s", i, rr.Code, rr.Body.String())
		}
		var tokResp map[string]any
		json.NewDecoder(rr.Body).Decode(&tokResp)
		tokens = append(tokens, tokResp["token"].(string))
	}

	// Send 2 requests via credential 0 — should succeed
	for i := 0; i < 2; i++ {
		proxyPath := "/v1/proxy/test"
		rr = h.proxyRequest(t, http.MethodGet, proxyPath, tokens[0], nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d via cred 0: expected 200, got %d: %s", i, rr.Code, rr.Body.String())
		}
	}

	// Send 1 request via credential 1 — should succeed (total = 3, at limit)
	proxyPath := "/v1/proxy/test"
	rr = h.proxyRequest(t, http.MethodGet, proxyPath, tokens[1], nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("request 3 via cred 1: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// 4th request (via either credential) should be rate limited
	rr = h.proxyRequest(t, http.MethodGet, proxyPath, tokens[1], nil)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("request 4: expected 429, got %d: %s", rr.Code, rr.Body.String())
	}

	// Also try via credential 0 — should also be blocked (shared counter)
	proxyPath0 := "/v1/proxy/test"
	rr = h.proxyRequest(t, http.MethodGet, proxyPath0, tokens[0], nil)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("request 5 via cred 0: expected 429 (shared limit), got %d: %s", rr.Code, rr.Body.String())
	}

	t.Logf("Shared rate limit enforced: 3 requests across 2 credentials, 4th and 5th blocked")
}

// --------------------------------------------------------------------------
// E2E: Credentials without identity skip identity rate limiting
// --------------------------------------------------------------------------

func TestE2E_Identity_NoIdentity_NoRateLimit(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer echoServer.Close()

	// Create credential WITHOUT identity
	cred := h.storeCredential(t, org, echoServer.URL, "bearer", "sk-no-identity")
	tok := h.mintToken(t, org, cred.ID)

	// Should be able to make many requests without identity rate limit
	for i := 0; i < 10; i++ {
		proxyPath := "/v1/proxy/test"
		rr := h.proxyRequest(t, http.MethodGet, proxyPath, tok, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200 (no identity rate limit), got %d", i, rr.Code)
		}
	}
}

// --------------------------------------------------------------------------
// E2E: Tenant isolation — org2 can't see org1's identities
// --------------------------------------------------------------------------

func TestE2E_Identity_TenantIsolation(t *testing.T) {
	h := newHarness(t)
	org1 := h.createOrg(t)
	org2 := h.createOrg(t)

	// Create identity in org1
	body := `{"external_id":"isolated_customer"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org1)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create identity org1: expected 201, got %d", rr.Code)
	}
	var identResp map[string]any
	json.NewDecoder(rr.Body).Decode(&identResp)
	identID := identResp["id"].(string)

	// org2 should NOT see it via GET
	req = httptest.NewRequest(http.MethodGet, "/v1/identities/"+identID, nil)
	req = middleware.WithOrg(req, &org2)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("org2 should not see org1's identity: expected 404, got %d", rr.Code)
	}

	// org2 should NOT see it via list
	req = httptest.NewRequest(http.MethodGet, "/v1/identities", nil)
	req = middleware.WithOrg(req, &org2)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list org2 identities: expected 200, got %d", rr.Code)
	}
	list := decodePaginatedList(t, rr)
	for _, ident := range list {
		if ident["id"] == identID {
			t.Fatal("org2 can see org1's identity — tenant isolation violated")
		}
	}

	// org2 should NOT be able to delete it
	req = httptest.NewRequest(http.MethodDelete, "/v1/identities/"+identID, nil)
	req = middleware.WithOrg(req, &org2)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("org2 delete org1's identity: expected 404, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// E2E: Identity deletion nullifies credential's identity_id
// --------------------------------------------------------------------------

func TestE2E_Identity_DeleteNullifiesCredentials(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create identity
	identBody := `{"external_id":"delete_test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(identBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create identity: expected 201, got %d", rr.Code)
	}
	var identResp map[string]any
	json.NewDecoder(rr.Body).Decode(&identResp)
	identID := identResp["id"].(string)

	// Create credential linked to identity
	credBody := fmt.Sprintf(`{"label":"linked","base_url":"https://api.example.com","auth_scheme":"bearer","api_key":"sk-test","identity_id":%q}`, identID)
	req = httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(credBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create credential: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var credResp map[string]any
	json.NewDecoder(rr.Body).Decode(&credResp)
	credID := credResp["id"].(string)

	// Delete identity
	req = httptest.NewRequest(http.MethodDelete, "/v1/identities/"+identID, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete identity: expected 200, got %d", rr.Code)
	}

	// Credential should still exist but with identity_id = null
	var cred model.Credential
	h.db.Where("id = ?", credID).First(&cred)
	if cred.IdentityID != nil {
		t.Fatalf("expected identity_id to be null after identity deletion, got %v", cred.IdentityID)
	}
}

// --------------------------------------------------------------------------
// E2E: Identity shared rate limit with live LLM proxy
// --------------------------------------------------------------------------

func TestE2E_Identity_SharedRateLimit_LiveLLM(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)

	t.Cleanup(func() {
		h.redisClient.FlushDB(context.Background())
	})

	// Create identity with limit: 2 requests per 60s
	identBody := `{"external_id":"live_rl_test","ratelimits":[{"name":"requests","limit":2,"duration":60000}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(identBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create identity: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var identResp map[string]any
	json.NewDecoder(rr.Body).Decode(&identResp)
	identID := identResp["id"].(string)

	// Create credential linked to identity, pointing to OpenRouter
	credBody := fmt.Sprintf(`{"label":"live-rl","base_url":"https://openrouter.ai/api","auth_scheme":"bearer","api_key":%q,"identity_id":%q}`, apiKey, identID)
	req = httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(credBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create credential: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var credResp map[string]any
	json.NewDecoder(rr.Body).Decode(&credResp)
	credID := credResp["id"].(string)

	// Mint token
	credUUID, _ := uuid.Parse(credID)
	tok := h.mintToken(t, org, credUUID)

	payload := `{
		"model": "openai/gpt-4.1-nano",
		"messages": [{"role": "user", "content": "Say hi"}],
		"stream": false,
		"max_tokens": 20
	}`

	proxyPath := "/v1/proxy/v1/chat/completions"

	// Request 1 — should succeed
	rr = h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload))
	if rr.Code != http.StatusOK {
		t.Fatalf("live request 1: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	t.Logf("Live request 1: OK")

	// Request 2 — should succeed
	rr = h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload))
	if rr.Code != http.StatusOK {
		t.Fatalf("live request 2: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	t.Logf("Live request 2: OK")

	// Request 3 — should be rate limited
	rr = h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload))
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("live request 3: expected 429, got %d: %s", rr.Code, rr.Body.String())
	}
	t.Logf("Live request 3: correctly rate limited (429)")
}

// --------------------------------------------------------------------------
// E2E: Request caps (remaining) with identity-linked credential via live LLM
// --------------------------------------------------------------------------

func TestE2E_Identity_RequestCaps_LiveLLM(t *testing.T) {
	apiKey := requireOpenRouterKey(t)
	h := newHarness(t)
	org := h.createOrg(t)

	t.Cleanup(func() {
		h.redisClient.FlushDB(context.Background())
	})

	// Create identity
	identBody := `{"external_id":"caps_test"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(identBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create identity: expected 201, got %d", rr.Code)
	}
	var identResp map[string]any
	json.NewDecoder(rr.Body).Decode(&identResp)
	identID := identResp["id"].(string)

	// Create credential with remaining=2 linked to identity
	credBody := fmt.Sprintf(`{"label":"capped","base_url":"https://openrouter.ai/api","auth_scheme":"bearer","api_key":%q,"identity_id":%q,"remaining":2}`, apiKey, identID)
	req = httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(credBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create credential: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var credResp map[string]any
	json.NewDecoder(rr.Body).Decode(&credResp)
	credID := credResp["id"].(string)

	credUUID, _ := uuid.Parse(credID)
	tok := h.mintToken(t, org, credUUID)

	payload := `{
		"model": "openai/gpt-4.1-nano",
		"messages": [{"role": "user", "content": "Say ok"}],
		"stream": false,
		"max_tokens": 20
	}`
	proxyPath := "/v1/proxy/v1/chat/completions"

	// Request 1 — OK
	rr = h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload))
	if rr.Code != http.StatusOK {
		t.Fatalf("capped request 1: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Request 2 — OK
	rr = h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload))
	if rr.Code != http.StatusOK {
		t.Fatalf("capped request 2: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Request 3 — should hit cap
	rr = h.proxyRequest(t, http.MethodPost, proxyPath, tok, strings.NewReader(payload))
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("capped request 3: expected 429, got %d: %s", rr.Code, rr.Body.String())
	}
	t.Logf("Request cap enforced: 2 allowed, 3rd blocked with 429")
}

// --------------------------------------------------------------------------
// E2E: Multiple named rate limits on a single identity
// --------------------------------------------------------------------------

func TestE2E_Identity_MultipleRateLimits(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	t.Cleanup(func() {
		h.redisClient.FlushDB(context.Background())
	})

	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer echoServer.Close()

	// Create identity with 2 named limits:
	// - "fast": 2 per 60s (will exhaust first)
	// - "slow": 10 per 60s (plenty of room)
	identBody := `{"external_id":"multi_rl","ratelimits":[{"name":"fast","limit":2,"duration":60000},{"name":"slow","limit":10,"duration":60000}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(identBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create identity: expected 201, got %d", rr.Code)
	}
	var identResp map[string]any
	json.NewDecoder(rr.Body).Decode(&identResp)
	identID := identResp["id"].(string)

	credBody := fmt.Sprintf(`{"label":"multi-rl","base_url":%q,"auth_scheme":"bearer","api_key":"sk-test","identity_id":%q}`, echoServer.URL, identID)
	req = httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(credBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create credential: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var credResp map[string]any
	json.NewDecoder(rr.Body).Decode(&credResp)
	credID := credResp["id"].(string)
	credUUID, _ := uuid.Parse(credID)
	tok := h.mintToken(t, org, credUUID)

	proxyPath := "/v1/proxy/test"

	// 2 requests OK
	for i := 0; i < 2; i++ {
		rr = h.proxyRequest(t, http.MethodGet, proxyPath, tok, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, rr.Code)
		}
	}

	// 3rd should fail on the "fast" limit
	rr = h.proxyRequest(t, http.MethodGet, proxyPath, tok, nil)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("request 3: expected 429, got %d: %s", rr.Code, rr.Body.String())
	}

	var errResp map[string]string
	json.NewDecoder(rr.Body).Decode(&errResp)
	if !strings.Contains(errResp["error"], "fast") {
		t.Logf("warning: expected error to mention 'fast' limit, got: %s", errResp["error"])
	}
	t.Logf("Multiple rate limits enforced: 'fast' limit (2/60s) blocked request 3")
}

// --------------------------------------------------------------------------
// E2E: Identity rate limit doesn't affect other org's proxy
// --------------------------------------------------------------------------

func TestE2E_Identity_RateLimit_OrgIsolation(t *testing.T) {
	h := newHarness(t)
	org1 := h.createOrg(t)
	org2 := h.createOrg(t)

	t.Cleanup(func() {
		h.redisClient.FlushDB(context.Background())
	})

	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer echoServer.Close()

	// Create identity with limit=1 in org1
	identBody := `{"external_id":"org_iso_test","ratelimits":[{"name":"requests","limit":1,"duration":60000}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(identBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org1)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create identity org1: expected 201, got %d", rr.Code)
	}
	var identResp map[string]any
	json.NewDecoder(rr.Body).Decode(&identResp)
	identID1 := identResp["id"].(string)

	// Create cred+token in org1 linked to identity
	credBody := fmt.Sprintf(`{"label":"org1","base_url":%q,"auth_scheme":"bearer","api_key":"sk-1","identity_id":%q}`, echoServer.URL, identID1)
	req = httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(credBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org1)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create cred org1: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var credResp map[string]any
	json.NewDecoder(rr.Body).Decode(&credResp)
	credID1 := credResp["id"].(string)
	credUUID1, _ := uuid.Parse(credID1)
	tok1 := h.mintToken(t, org1, credUUID1)

	// Create cred+token in org2 WITHOUT identity (no rate limit)
	cred2 := h.storeCredential(t, org2, echoServer.URL, "bearer", "sk-2")
	tok2 := h.mintToken(t, org2, cred2.ID)

	// Exhaust org1's identity limit
	proxyPath1 := "/v1/proxy/test"
	rr = h.proxyRequest(t, http.MethodGet, proxyPath1, tok1, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("org1 request 1: expected 200, got %d", rr.Code)
	}
	rr = h.proxyRequest(t, http.MethodGet, proxyPath1, tok1, nil)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("org1 request 2: expected 429, got %d", rr.Code)
	}

	// org2 should still work fine — different identity/counter
	proxyPath2 := "/v1/proxy/test"
	for i := 0; i < 5; i++ {
		rr = h.proxyRequest(t, http.MethodGet, proxyPath2, tok2, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("org2 request %d: expected 200 (no rate limit), got %d", i+1, rr.Code)
		}
	}
	t.Logf("Org isolation verified: org1 rate limited, org2 unaffected")
}

// --------------------------------------------------------------------------
// E2E: Token with remaining + identity linked credential via local echo
// --------------------------------------------------------------------------

func TestE2E_Identity_TokenCap_WithIdentity(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	t.Cleanup(func() {
		h.redisClient.FlushDB(context.Background())
	})

	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer echoServer.Close()

	// Create identity with generous rate limit (won't interfere)
	identBody := `{"external_id":"token_cap_test","ratelimits":[{"name":"requests","limit":100,"duration":60000}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(identBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create identity: expected 201, got %d", rr.Code)
	}
	var identResp map[string]any
	json.NewDecoder(rr.Body).Decode(&identResp)
	identID := identResp["id"].(string)

	// Create credential linked to identity
	credBody := fmt.Sprintf(`{"label":"tok-cap","base_url":%q,"auth_scheme":"bearer","api_key":"sk-test","identity_id":%q}`, echoServer.URL, identID)
	req = httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(credBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create credential: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var credResp map[string]any
	json.NewDecoder(rr.Body).Decode(&credResp)
	credID := credResp["id"].(string)

	// Mint token with remaining=2
	tokBody := fmt.Sprintf(`{"credential_id":%q,"ttl":"1h","remaining":2}`, credID)
	req = httptest.NewRequest(http.MethodPost, "/v1/tokens", strings.NewReader(tokBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("mint token: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var tokResp map[string]any
	json.NewDecoder(rr.Body).Decode(&tokResp)
	tokStr := tokResp["token"].(string)

	proxyPath := "/v1/proxy/test"

	// 2 requests OK
	for i := 0; i < 2; i++ {
		rr = h.proxyRequest(t, http.MethodGet, proxyPath, tokStr, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d: %s", i+1, rr.Code, rr.Body.String())
		}
	}

	// 3rd should hit token cap
	rr = h.proxyRequest(t, http.MethodGet, proxyPath, tokStr, nil)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("request 3: expected 429, got %d: %s", rr.Code, rr.Body.String())
	}

	// Mint a NEW token (no cap) for the same credential — should work
	credUUID, _ := uuid.Parse(credID)
	tokenStr2, jti2, _ := token.Mint(h.signingKey, org.ID.String(), credID, time.Hour)
	h.db.Create(&model.Token{
		ID: uuid.New(), OrgID: org.ID, CredentialID: credUUID,
		JTI: jti2, ExpiresAt: time.Now().Add(time.Hour),
	})
	rr = h.proxyRequest(t, http.MethodGet, proxyPath, "ptok_"+tokenStr2, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("new token request: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	t.Logf("Token cap enforced: 2 via capped token, 3rd blocked, new uncapped token works")
}

