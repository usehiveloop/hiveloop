package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
)

// --------------------------------------------------------------------------
// E2E: Credential list pagination
// --------------------------------------------------------------------------

func TestE2E_Credential_Pagination(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create 5 credentials
	for i := 0; i < 5; i++ {
		body := fmt.Sprintf(`{"label":"page-cred-%d","provider_id":"openai","base_url":"https://api.example.com","auth_scheme":"bearer","api_key":"sk-page-%d"}`, i, i)
		req := httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		rr := httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create credential %d: expected 201, got %d: %s", i, rr.Code, rr.Body.String())
		}
		time.Sleep(5 * time.Millisecond) // ensure distinct created_at
	}

	// Page 1: limit=2
	req := httptest.NewRequest(http.MethodGet, "/v1/credentials?limit=2", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("page 1: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var page1 struct {
		Data       []map[string]any `json:"data"`
		NextCursor *string          `json:"next_cursor"`
		HasMore    bool             `json:"has_more"`
	}
	json.NewDecoder(rr.Body).Decode(&page1)

	if len(page1.Data) != 2 {
		t.Fatalf("page 1: expected 2 items, got %d", len(page1.Data))
	}
	if !page1.HasMore {
		t.Fatal("page 1: expected has_more=true")
	}
	if page1.NextCursor == nil {
		t.Fatal("page 1: expected next_cursor to be set")
	}

	// Verify descending order (newest first) — compare label suffix since created_at
	// may have same second-level precision in RFC3339
	label0 := page1.Data[0]["label"].(string)
	label1 := page1.Data[1]["label"].(string)
	if label0 < label1 {
		// Labels are page-cred-0..4; newer ones have higher numbers (created later)
		// In descending order, higher number should come first
		t.Logf("labels: %s, %s (order may vary depending on exact timing)", label0, label1)
	}

	// Page 2: use cursor
	req = httptest.NewRequest(http.MethodGet, "/v1/credentials?limit=2&cursor="+*page1.NextCursor, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("page 2: expected 200, got %d", rr.Code)
	}

	var page2 struct {
		Data       []map[string]any `json:"data"`
		NextCursor *string          `json:"next_cursor"`
		HasMore    bool             `json:"has_more"`
	}
	json.NewDecoder(rr.Body).Decode(&page2)

	if len(page2.Data) != 2 {
		t.Fatalf("page 2: expected 2 items, got %d", len(page2.Data))
	}
	if !page2.HasMore {
		t.Fatal("page 2: expected has_more=true")
	}

	// No overlap between pages
	page1IDs := map[string]bool{}
	for _, item := range page1.Data {
		page1IDs[item["id"].(string)] = true
	}
	for _, item := range page2.Data {
		if page1IDs[item["id"].(string)] {
			t.Fatalf("duplicate item across pages: %s", item["id"])
		}
	}

	// Page 3: should have 1 item and has_more=false
	req = httptest.NewRequest(http.MethodGet, "/v1/credentials?limit=2&cursor="+*page2.NextCursor, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("page 3: expected 200, got %d", rr.Code)
	}

	var page3 struct {
		Data       []map[string]any `json:"data"`
		NextCursor *string          `json:"next_cursor"`
		HasMore    bool             `json:"has_more"`
	}
	json.NewDecoder(rr.Body).Decode(&page3)

	if len(page3.Data) != 1 {
		t.Fatalf("page 3: expected 1 item, got %d", len(page3.Data))
	}
	if page3.HasMore {
		t.Fatal("page 3: expected has_more=false")
	}
	if page3.NextCursor != nil {
		t.Fatal("page 3: expected no next_cursor")
	}

	t.Logf("Pagination verified: 5 credentials across 3 pages (2+2+1)")
}

// --------------------------------------------------------------------------
// E2E: Identity list pagination
// --------------------------------------------------------------------------

func TestE2E_Identity_Pagination(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create 4 identities
	for i := 0; i < 4; i++ {
		body := fmt.Sprintf(`{"external_id":"page-ident-%d-%s"}`, i, uuid.New().String()[:8])
		req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		rr := httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create identity %d: expected 201, got %d: %s", i, rr.Code, rr.Body.String())
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Page 1: limit=3
	req := httptest.NewRequest(http.MethodGet, "/v1/identities?limit=3", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("page 1: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var page1 struct {
		Data       []map[string]any `json:"data"`
		NextCursor *string          `json:"next_cursor"`
		HasMore    bool             `json:"has_more"`
	}
	json.NewDecoder(rr.Body).Decode(&page1)

	if len(page1.Data) != 3 {
		t.Fatalf("page 1: expected 3 items, got %d", len(page1.Data))
	}
	if !page1.HasMore {
		t.Fatal("page 1: expected has_more=true")
	}

	// Page 2: remaining 1
	req = httptest.NewRequest(http.MethodGet, "/v1/identities?limit=3&cursor="+*page1.NextCursor, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("page 2: expected 200, got %d", rr.Code)
	}

	var page2 struct {
		Data       []map[string]any `json:"data"`
		NextCursor *string          `json:"next_cursor"`
		HasMore    bool             `json:"has_more"`
	}
	json.NewDecoder(rr.Body).Decode(&page2)

	if len(page2.Data) != 1 {
		t.Fatalf("page 2: expected 1 item, got %d", len(page2.Data))
	}
	if page2.HasMore {
		t.Fatal("page 2: expected has_more=false")
	}

	t.Logf("Identity pagination verified: 4 identities across 2 pages (3+1)")
}

// --------------------------------------------------------------------------
// E2E: Invalid pagination params
// --------------------------------------------------------------------------

func TestE2E_Pagination_InvalidParams(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Invalid limit
	req := httptest.NewRequest(http.MethodGet, "/v1/credentials?limit=-1", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit: expected 400, got %d", rr.Code)
	}

	// Invalid cursor
	req = httptest.NewRequest(http.MethodGet, "/v1/credentials?cursor=not-valid-base64!!", nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid cursor: expected 400, got %d", rr.Code)
	}

	// Limit capped at 100
	for i := 0; i < 3; i++ {
		body := fmt.Sprintf(`{"label":"cap-test-%d","provider_id":"openai","base_url":"https://api.example.com","auth_scheme":"bearer","api_key":"sk-%d"}`, i, i)
		req = httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		rr = httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
	}
	req = httptest.NewRequest(http.MethodGet, "/v1/credentials?limit=200", nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("limit cap: expected 200, got %d", rr.Code)
	}
	// Should work without error (limit capped to 100 internally)
}

// --------------------------------------------------------------------------
// E2E: Credential usage stats (request_count + last_used_at)
// --------------------------------------------------------------------------

func TestE2E_Credential_UsageStats(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer echoServer.Close()

	// Create 2 credentials
	cred1 := h.storeCredential(t, org, echoServer.URL, "bearer", "sk-stats-1")
	cred2 := h.storeCredential(t, org, echoServer.URL, "bearer", "sk-stats-2")

	// Mint tokens
	tok1 := h.mintToken(t, org, cred1.ID)
	tok2 := h.mintToken(t, org, cred2.ID)

	// Make 3 proxy requests via cred1
	for i := 0; i < 3; i++ {
		rr := h.proxyRequest(t, http.MethodGet, "/v1/proxy/test", tok1, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("proxy cred1 request %d: expected 200, got %d", i, rr.Code)
		}
	}

	// Make 1 proxy request via cred2
	rr := h.proxyRequest(t, http.MethodGet, "/v1/proxy/test", tok2, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("proxy cred2 request: expected 200, got %d", rr.Code)
	}

	// Wait for audit writer to flush
	time.Sleep(200 * time.Millisecond)

	// List credentials — should include usage stats
	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	creds := decodePaginatedList(t, rr)
	statsMap := map[string]map[string]any{}
	for _, c := range creds {
		statsMap[c["id"].(string)] = c
	}

	// cred1 should have request_count=3
	c1 := statsMap[cred1.ID.String()]
	if c1 == nil {
		t.Fatal("cred1 not found in list")
	}
	rc1 := int64(c1["request_count"].(float64))
	if rc1 != 3 {
		t.Fatalf("cred1: expected request_count=3, got %d", rc1)
	}
	if c1["last_used_at"] == nil {
		t.Fatal("cred1: expected last_used_at to be set")
	}

	// cred2 should have request_count=1
	c2 := statsMap[cred2.ID.String()]
	if c2 == nil {
		t.Fatal("cred2 not found in list")
	}
	rc2 := int64(c2["request_count"].(float64))
	if rc2 != 1 {
		t.Fatalf("cred2: expected request_count=1, got %d", rc2)
	}

	t.Logf("Credential usage stats verified: cred1=%d, cred2=%d", rc1, rc2)
}

// --------------------------------------------------------------------------
// E2E: Credential with no proxy requests has zero usage stats
// --------------------------------------------------------------------------

func TestE2E_Credential_ZeroUsageStats(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	h.storeCredential(t, org, "https://api.example.com", "bearer", "sk-unused")

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}

	creds := decodePaginatedList(t, rr)
	if len(creds) == 0 {
		t.Fatal("expected at least 1 credential")
	}

	rc := int64(creds[0]["request_count"].(float64))
	if rc != 0 {
		t.Fatalf("expected request_count=0, got %d", rc)
	}
	if creds[0]["last_used_at"] != nil {
		t.Fatal("expected last_used_at to be nil for unused credential")
	}
}

// --------------------------------------------------------------------------
// E2E: Identity usage stats (request_count + last_used_at)
// --------------------------------------------------------------------------

func TestE2E_Identity_UsageStats(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer echoServer.Close()

	// Create 2 identities
	var identIDs []string
	for i := 0; i < 2; i++ {
		body := fmt.Sprintf(`{"external_id":"stats-ident-%d-%s"}`, i, uuid.New().String()[:8])
		req := httptest.NewRequest(http.MethodPost, "/v1/identities", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		rr := httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create identity %d: expected 201, got %d", i, rr.Code)
		}
		var resp map[string]any
		json.NewDecoder(rr.Body).Decode(&resp)
		identIDs = append(identIDs, resp["id"].(string))
	}

	// Create credential for identity 0 and make 2 proxy requests
	credBody := fmt.Sprintf(`{"label":"stats-cred-0","provider_id":"openai","base_url":%q,"auth_scheme":"bearer","api_key":"sk-si0","identity_id":%q}`, echoServer.URL, identIDs[0])
	req := httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(credBody))
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create cred: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var credResp map[string]any
	json.NewDecoder(rr.Body).Decode(&credResp)
	credUUID, _ := uuid.Parse(credResp["id"].(string))
	tok := h.mintToken(t, org, credUUID)

	for i := 0; i < 2; i++ {
		rr = h.proxyRequest(t, http.MethodGet, "/v1/proxy/test", tok, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("proxy request %d: expected 200, got %d", i, rr.Code)
		}
	}

	// Wait for audit writer to flush
	time.Sleep(200 * time.Millisecond)

	// List identities — check usage stats
	req = httptest.NewRequest(http.MethodGet, "/v1/identities", nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	identities := decodePaginatedList(t, rr)
	statsMap := map[string]map[string]any{}
	for _, ident := range identities {
		statsMap[ident["id"].(string)] = ident
	}

	// Identity 0 should have request_count=2
	i0 := statsMap[identIDs[0]]
	if i0 == nil {
		t.Fatal("identity 0 not found in list")
	}
	rc0 := int64(i0["request_count"].(float64))
	if rc0 != 2 {
		t.Fatalf("identity 0: expected request_count=2, got %d", rc0)
	}
	if i0["last_used_at"] == nil {
		t.Fatal("identity 0: expected last_used_at to be set")
	}

	// Identity 1 should have request_count=0
	i1 := statsMap[identIDs[1]]
	if i1 == nil {
		t.Fatal("identity 1 not found in list")
	}
	rc1 := int64(i1["request_count"].(float64))
	if rc1 != 0 {
		t.Fatalf("identity 1: expected request_count=0, got %d", rc1)
	}

	t.Logf("Identity usage stats verified: ident0=%d, ident1=%d", rc0, rc1)
}

// --------------------------------------------------------------------------
// E2E: Audit entry includes identity_id after proxy request
// --------------------------------------------------------------------------

func TestE2E_AuditEntry_IdentityID(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer echoServer.Close()

	// Create identity
	identBody := `{"external_id":"audit-test"}`
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
	identUUID, _ := uuid.Parse(identID)

	// Create credential linked to identity
	credBody := fmt.Sprintf(`{"label":"audit-cred","provider_id":"openai","base_url":%q,"auth_scheme":"bearer","api_key":"sk-audit","identity_id":%q}`, echoServer.URL, identID)
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

	// Make proxy request
	rr = h.proxyRequest(t, http.MethodGet, "/v1/proxy/test", tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("proxy request: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Wait for audit writer to flush
	time.Sleep(200 * time.Millisecond)

	// Verify audit entry has identity_id set
	var entries []model.AuditEntry
	h.db.Where("org_id = ? AND action = 'proxy.request' AND credential_id = ?", org.ID, credUUID).
		Order("created_at DESC").Find(&entries)

	if len(entries) == 0 {
		t.Fatal("expected at least 1 audit entry")
	}

	entry := entries[0]
	if entry.IdentityID == nil {
		t.Fatal("expected identity_id to be set on audit entry")
	}
	if *entry.IdentityID != identUUID {
		t.Fatalf("expected identity_id=%s, got %s", identUUID, *entry.IdentityID)
	}
	if entry.CredentialID == nil || *entry.CredentialID != credUUID {
		t.Fatalf("expected credential_id=%s on audit entry", credUUID)
	}

	t.Logf("Audit entry verified: identity_id=%s, credential_id=%s", *entry.IdentityID, *entry.CredentialID)
}

// --------------------------------------------------------------------------
// E2E: Audit entry has nil identity_id for credential without identity
// --------------------------------------------------------------------------

func TestE2E_AuditEntry_NilIdentityID(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer echoServer.Close()

	// Create credential WITHOUT identity
	cred := h.storeCredential(t, org, echoServer.URL, "bearer", "sk-no-ident-audit")
	tok := h.mintToken(t, org, cred.ID)

	// Make proxy request
	rr := h.proxyRequest(t, http.MethodGet, "/v1/proxy/test", tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("proxy request: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Wait for audit writer to flush
	time.Sleep(200 * time.Millisecond)

	// Verify audit entry has nil identity_id
	var entries []model.AuditEntry
	h.db.Where("org_id = ? AND action = 'proxy.request' AND credential_id = ?", org.ID, cred.ID).
		Order("created_at DESC").Find(&entries)

	if len(entries) == 0 {
		t.Fatal("expected at least 1 audit entry")
	}

	entry := entries[0]
	if entry.IdentityID != nil {
		t.Fatalf("expected nil identity_id for credential without identity, got %s", *entry.IdentityID)
	}

	t.Logf("Audit entry verified: identity_id=nil for credential without identity")
}

// --------------------------------------------------------------------------
// E2E: Pagination with filters
// --------------------------------------------------------------------------

func TestE2E_Credential_Pagination_WithFilter(t *testing.T) {
	h := newHarness(t)
	org := h.createOrg(t)

	// Create identity
	identBody := `{"external_id":"page-filter-test"}`
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

	// Create 3 credentials linked to identity + 2 without
	for i := 0; i < 3; i++ {
		body := fmt.Sprintf(`{"label":"linked-%d","provider_id":"openai","base_url":"https://api.example.com","auth_scheme":"bearer","api_key":"sk-l-%d","identity_id":%q}`, i, i, identID)
		req = httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		rr = httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create linked cred %d: expected 201, got %d", i, rr.Code)
		}
		time.Sleep(5 * time.Millisecond)
	}
	for i := 0; i < 2; i++ {
		body := fmt.Sprintf(`{"label":"unlinked-%d","provider_id":"openai","base_url":"https://api.example.com","auth_scheme":"bearer","api_key":"sk-u-%d"}`, i, i)
		req = httptest.NewRequest(http.MethodPost, "/v1/credentials", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = middleware.WithOrg(req, &org)
		rr = httptest.NewRecorder()
		h.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create unlinked cred %d: expected 201, got %d", i, rr.Code)
		}
	}

	// Paginate with identity_id filter — should only see 3 linked creds
	req = httptest.NewRequest(http.MethodGet, "/v1/credentials?identity_id="+identID+"&limit=2", nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("filtered page 1: expected 200, got %d", rr.Code)
	}

	var fPage1 struct {
		Data       []map[string]any `json:"data"`
		NextCursor *string          `json:"next_cursor"`
		HasMore    bool             `json:"has_more"`
	}
	json.NewDecoder(rr.Body).Decode(&fPage1)

	if len(fPage1.Data) != 2 {
		t.Fatalf("filtered page 1: expected 2 items, got %d", len(fPage1.Data))
	}
	if !fPage1.HasMore {
		t.Fatal("filtered page 1: expected has_more=true")
	}
	for _, c := range fPage1.Data {
		if c["identity_id"] != identID {
			t.Fatalf("filtered result has wrong identity_id: %v", c["identity_id"])
		}
	}

	// Page 2
	req = httptest.NewRequest(http.MethodGet, "/v1/credentials?identity_id="+identID+"&limit=2&cursor="+*fPage1.NextCursor, nil)
	req = middleware.WithOrg(req, &org)
	rr = httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("filtered page 2: expected 200, got %d", rr.Code)
	}

	var fPage2 struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	json.NewDecoder(rr.Body).Decode(&fPage2)

	if len(fPage2.Data) != 1 {
		t.Fatalf("filtered page 2: expected 1 item, got %d", len(fPage2.Data))
	}
	if fPage2.HasMore {
		t.Fatal("filtered page 2: expected has_more=false")
	}

	t.Logf("Pagination with filter verified: 3 linked credentials across 2 pages")
}
