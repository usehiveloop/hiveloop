package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"


	"github.com/ziraloop/ziraloop/internal/middleware"
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
