package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func TestAPIKeyHandler_Create_Success(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "prod-key",
		"scopes": []string{"connect", "credentials"},
	}, &org)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	key, ok := resp["key"].(string)
	if !ok || !strings.HasPrefix(key, "hvl_sk_") {
		t.Fatalf("expected key with hvl_sk_ prefix, got %v", resp["key"])
	}
	if len(key) != 71 {
		t.Fatalf("expected key length 71, got %d", len(key))
	}

	prefix, ok := resp["key_prefix"].(string)
	if !ok || len(prefix) != 16 {
		t.Fatalf("expected key_prefix length 16, got %v", resp["key_prefix"])
	}

	scopes, ok := resp["scopes"].([]any)
	if !ok || len(scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %v", resp["scopes"])
	}

	if resp["name"] != "prod-key" {
		t.Fatalf("expected name 'prod-key', got %v", resp["name"])
	}

	var dbKey model.APIKey
	if err := h.db.Where("id = ?", resp["id"]).First(&dbKey).Error; err != nil {
		t.Fatalf("key not found in DB: %v", err)
	}
	if dbKey.OrgID != org.ID {
		t.Fatalf("expected org ID %s, got %s", org.ID, dbKey.OrgID)
	}

	expectedHash := model.HashAPIKey(key)
	if dbKey.KeyHash != expectedHash {
		t.Fatalf("stored hash does not match: expected %q, got %q", expectedHash, dbKey.KeyHash)
	}
}

func TestAPIKeyHandler_Create_WithExpiry(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":       "expiring-key",
		"scopes":     []string{"all"},
		"expires_in": "720h",
	}, &org)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	expiresAt, ok := resp["expires_at"].(string)
	if !ok || expiresAt == "" {
		t.Fatal("expected expires_at in response")
	}

	expTime, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		t.Fatalf("parse expires_at: %v", err)
	}
	diff := time.Until(expTime)
	if diff < 719*time.Hour || diff > 721*time.Hour {
		t.Fatalf("expected expiry ~720h from now, got %v", diff)
	}
}

func TestAPIKeyHandler_Create_MissingName(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"scopes": []string{"all"},
	}, &org)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAPIKeyHandler_Create_MissingScopes(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name": "no-scopes",
	}, &org)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAPIKeyHandler_Create_InvalidScope(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "bad-scope",
		"scopes": []string{"admin"},
	}, &org)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if !strings.Contains(body["error"], "invalid scope") {
		t.Fatalf("expected invalid scope error, got %q", body["error"])
	}
}

func TestAPIKeyHandler_Create_InvalidExpiresIn(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":       "bad-expiry",
		"scopes":     []string{"all"},
		"expires_in": "not-a-duration",
	}, &org)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestAPIKeyHandler_Create_MissingOrg(t *testing.T) {
	h := newAPIKeyHarness(t)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "no-org",
		"scopes": []string{"all"},
	}, nil)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
