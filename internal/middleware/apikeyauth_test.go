package middleware_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_APIKeyAuth_ValidKey(t *testing.T) {
	db := connectTestDB(t)
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)

	orgID := uuid.New()
	org := model.Org{
		ID:   orgID,
		Name: fmt.Sprintf("apikey-valid-%s", uuid.New().String()[:8]),

		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", orgID).Delete(&model.APIKey{})
		cleanupOrg(t, db, orgID)
	})

	// Generate and store an API key
	plaintext, hash, prefix, err := model.GenerateAPIKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	apiKey := model.APIKey{
		ID:        uuid.New(),
		OrgID:     orgID,
		Name:      "test-key",
		KeyHash:   hash,
		KeyPrefix: prefix,
		Scopes:    []string{"all"},
	}
	if err := db.Create(&apiKey).Error; err != nil {
		t.Fatalf("failed to create api key: %v", err)
	}

	var gotOrg *model.Org
	var gotClaims *middleware.APIKeyClaims
	handler := middleware.APIKeyAuth(db, keyCache, &enqueue.MockClient{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		gotOrg, ok = middleware.OrgFromContext(r.Context())
		if !ok {
			t.Fatal("org not found in context")
		}
		gotClaims, ok = middleware.APIKeyClaimsFromContext(r.Context())
		if !ok {
			t.Fatal("api key claims not found in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if gotOrg == nil || gotOrg.ID != orgID {
		t.Fatalf("expected org ID %s, got %v", orgID, gotOrg)
	}
	if gotClaims.KeyID != apiKey.ID.String() {
		t.Fatalf("expected key ID %s, got %s", apiKey.ID, gotClaims.KeyID)
	}
	if gotClaims.OrgID != orgID.String() {
		t.Fatalf("expected org ID %s in claims, got %s", orgID, gotClaims.OrgID)
	}
	if len(gotClaims.Scopes) != 1 || gotClaims.Scopes[0] != "all" {
		t.Fatalf("expected scopes [all], got %v", gotClaims.Scopes)
	}
}

func TestIntegration_APIKeyAuth_CacheHit(t *testing.T) {
	db := connectTestDB(t)
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)

	orgID := uuid.New()
	org := model.Org{
		ID:   orgID,
		Name: fmt.Sprintf("apikey-cache-%s", uuid.New().String()[:8]),

		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", orgID).Delete(&model.APIKey{})
		cleanupOrg(t, db, orgID)
	})

	plaintext, hash, prefix, err := model.GenerateAPIKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	apiKey := model.APIKey{
		ID:        uuid.New(),
		OrgID:     orgID,
		Name:      "test-key-cache",
		KeyHash:   hash,
		KeyPrefix: prefix,
		Scopes:    []string{"connect"},
	}
	if err := db.Create(&apiKey).Error; err != nil {
		t.Fatalf("failed to create api key: %v", err)
	}

	handler := middleware.APIKeyAuth(db, keyCache, &enqueue.MockClient{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request: cache miss, DB lookup.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rr.Code)
	}

	// Verify it's now cached
	cached, ok := keyCache.Get(hash)
	if !ok {
		t.Fatal("expected key to be cached after first request")
	}
	if cached.ID != apiKey.ID {
		t.Fatalf("cached ID mismatch: expected %s, got %s", apiKey.ID, cached.ID)
	}

	// Second request: cache hit.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Authorization", "Bearer "+plaintext)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("second request (cache hit): expected 200, got %d", rr2.Code)
	}
}

func TestIntegration_APIKeyAuth_InvalidKey(t *testing.T) {
	db := connectTestDB(t)
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)

	handler := middleware.APIKeyAuth(db, keyCache, &enqueue.MockClient{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for invalid key")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer hvl_sk_0000000000000000000000000000000000000000000000000000000000000000")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestIntegration_APIKeyAuth_WrongPrefix(t *testing.T) {
	db := connectTestDB(t)
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)

	handler := middleware.APIKeyAuth(db, keyCache, &enqueue.MockClient{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for wrong prefix")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer sk_test_wrongprefix123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestIntegration_APIKeyAuth_MissingAuth(t *testing.T) {
	db := connectTestDB(t)
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)

	handler := middleware.APIKeyAuth(db, keyCache, &enqueue.MockClient{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called without auth")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
