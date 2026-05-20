package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/model"
)

func TestAPIKeyHandler_Revoke_Success(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "to-revoke",
		"scopes": []string{"all"},
	}, &org)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", rr.Code)
	}
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)

	rr = h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+created["id"].(string), nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["status"] != "revoked" {
		t.Fatalf("expected status 'revoked', got %q", body["status"])
	}

	var dbKey model.APIKey
	if err := h.db.Where("id = ?", created["id"]).First(&dbKey).Error; err != nil {
		t.Fatalf("key not found: %v", err)
	}
	if dbKey.RevokedAt == nil {
		t.Fatal("expected revoked_at to be set")
	}
}

func TestAPIKeyHandler_Revoke_InvalidatesCache(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "cache-test",
		"scopes": []string{"all"},
	}, &org)
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)

	keyHash := model.HashAPIKey(created["key"].(string))
	h.cache.Set(keyHash, &cache.CachedAPIKey{
		ID:     uuid.MustParse(created["id"].(string)),
		OrgID:  org.ID,
		Scopes: []string{"all"},
	})

	if _, ok := h.cache.Get(keyHash); !ok {
		t.Fatal("expected key to be cached before revoke")
	}

	h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+created["id"].(string), nil, &org)

	if _, ok := h.cache.Get(keyHash); ok {
		t.Fatal("expected key to be evicted from cache after revoke")
	}
}

func TestAPIKeyHandler_Revoke_NotFound(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+uuid.New().String(), nil, &org)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestAPIKeyHandler_Revoke_AlreadyRevoked(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "double-revoke",
		"scopes": []string{"all"},
	}, &org)
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)

	h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+created["id"].(string), nil, &org)

	rr = h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+created["id"].(string), nil, &org)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for already-revoked key, got %d", rr.Code)
	}
}

func TestAPIKeyHandler_Revoke_WrongOrg(t *testing.T) {
	h := newAPIKeyHarness(t)
	org1 := createTestOrg(t, h.db)
	org2 := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "org1-secret",
		"scopes": []string{"all"},
	}, &org1)
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)

	rr = h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+created["id"].(string), nil, &org2)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (wrong org), got %d", rr.Code)
	}

	var dbKey model.APIKey
	h.db.Where("id = ?", created["id"]).First(&dbKey)
	if dbKey.RevokedAt != nil {
		t.Fatal("key should not be revoked by wrong org")
	}
}

func TestAPIKeyHandler_Revoke_MissingOrg(t *testing.T) {
	h := newAPIKeyHarness(t)

	rr := h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+uuid.New().String(), nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
