package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func TestAPIKeyHandler_FullLifecycle(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "lifecycle-key",
		"scopes": []string{"connect", "tokens"},
	}, &org)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)

	keyID := created["id"].(string)
	plaintext := created["key"].(string)

	rr = h.doRequest(t, http.MethodGet, "/v1/api-keys", nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rr.Code)
	}
	var listPage struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&listPage)

	found := false
	for _, k := range listPage.Data {
		if k["id"] == keyID {
			found = true
			if k["name"] != "lifecycle-key" {
				t.Fatalf("expected name 'lifecycle-key', got %v", k["name"])
			}
			if _, hasPlaintext := k["key"]; hasPlaintext {
				t.Fatal("plaintext key must NOT appear in list")
			}
			if k["revoked_at"] != nil {
				t.Fatal("key should not be revoked yet")
			}
		}
	}
	if !found {
		t.Fatal("created key not found in list")
	}

	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)
	var authedOrg *model.Org
	authHandler := middleware.APIKeyAuth(h.db, keyCache, &enqueue.MockClient{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		authedOrg, ok = middleware.OrgFromContext(r.Context())
		if !ok {
			t.Fatal("org not in context during auth")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	authRR := httptest.NewRecorder()
	authHandler.ServeHTTP(authRR, req)
	if authRR.Code != http.StatusOK {
		t.Fatalf("auth: expected 200, got %d; body: %s", authRR.Code, authRR.Body.String())
	}
	if authedOrg.ID != org.ID {
		t.Fatalf("auth: expected org %s, got %s", org.ID, authedOrg.ID)
	}

	rr = h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+keyID, nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d", rr.Code)
	}

	keyCache2 := cache.NewAPIKeyCache(100, 5*time.Minute)
	authHandler2 := middleware.APIKeyAuth(h.db, keyCache2, &enqueue.MockClient{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for revoked key")
	}))

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Authorization", "Bearer "+plaintext)
	authRR2 := httptest.NewRecorder()
	authHandler2.ServeHTTP(authRR2, req2)
	if authRR2.Code != http.StatusUnauthorized {
		t.Fatalf("auth after revoke: expected 401, got %d", authRR2.Code)
	}

	rr = h.doRequest(t, http.MethodGet, "/v1/api-keys", nil, &org)
	var afterPage struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&afterPage)

	for _, k := range afterPage.Data {
		if k["id"] == keyID {
			if k["revoked_at"] == nil {
				t.Fatal("expected revoked_at to be set after revocation")
			}
		}
	}
}
