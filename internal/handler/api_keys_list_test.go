package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestAPIKeyHandler_List_ReturnsKeys(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	for _, name := range []string{"key-alpha", "key-beta"} {
		rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
			"name":   name,
			"scopes": []string{"all"},
		}, &org)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create %s: expected 201, got %d", name, rr.Code)
		}
	}

	rr := h.doRequest(t, http.MethodGet, "/v1/api-keys", nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var page struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&page); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(page.Data) < 2 {
		t.Fatalf("expected at least 2 keys, got %d", len(page.Data))
	}

	for _, k := range page.Data {
		if _, hasKey := k["key"]; hasKey {
			t.Fatal("list response should NOT include plaintext key")
		}
		if _, hasPrefix := k["key_prefix"]; !hasPrefix {
			t.Fatal("list response should include key_prefix")
		}
	}
}

func TestAPIKeyHandler_List_IsolatedByOrg(t *testing.T) {
	h := newAPIKeyHarness(t)
	org1 := createTestOrg(t, h.db)
	org2 := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "org1-key",
		"scopes": []string{"all"},
	}, &org1)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", rr.Code)
	}

	rr = h.doRequest(t, http.MethodGet, "/v1/api-keys", nil, &org2)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	for _, k := range page.Data {
		if k["name"] == "org1-key" {
			t.Fatal("org2 should not see org1's keys")
		}
	}
}

func TestAPIKeyHandler_List_Empty(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodGet, "/v1/api-keys", nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var page struct {
		Data    []map[string]any `json:"data"`
		HasMore bool             `json:"has_more"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)
	if len(page.Data) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(page.Data))
	}
	if page.HasMore {
		t.Fatal("expected has_more=false for empty list")
	}
}

func TestAPIKeyHandler_List_OrderedByCreatedDesc(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	names := []string{"first", "second", "third"}
	for _, name := range names {
		rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
			"name":   name,
			"scopes": []string{"all"},
		}, &org)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create %s: expected 201, got %d", name, rr.Code)
		}

		time.Sleep(10 * time.Millisecond)
	}

	rr := h.doRequest(t, http.MethodGet, "/v1/api-keys", nil, &org)
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)

	if len(page.Data) < 3 {
		t.Fatalf("expected at least 3 keys, got %d", len(page.Data))
	}

	if page.Data[0]["name"] != "third" {
		t.Fatalf("expected first entry to be 'third' (newest), got %v", page.Data[0]["name"])
	}
}

func TestAPIKeyHandler_List_IncludesRevokedKeys(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "will-revoke",
		"scopes": []string{"all"},
	}, &org)
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)

	h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+created["id"].(string), nil, &org)

	rr = h.doRequest(t, http.MethodGet, "/v1/api-keys", nil, &org)
	var page struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&page)

	found := false
	for _, k := range page.Data {
		if k["id"] == created["id"] {
			found = true
			if k["revoked_at"] == nil {
				t.Fatal("expected revoked_at to be set on revoked key")
			}
		}
	}
	if !found {
		t.Fatal("revoked key should appear in list")
	}
}
