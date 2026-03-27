package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/cache"
	"github.com/llmvault/llmvault/internal/handler"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
)

const (
	testDBURL     = "postgres://llmvault:localdev@localhost:5433/llmvault_test?sslmode=disable"
	testRedisAddr = "localhost:6379"
)

func connectTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = testDBURL
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("cannot connect to Postgres: %v", err)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("Postgres not reachable: %v", err)
	}
	if err := model.AutoMigrate(db); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	return db
}

func connectTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: testRedisAddr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("Redis not reachable: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func cleanupOrg(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	db.Where("org_id = ?", orgID).Delete(&model.Generation{})
	db.Where("org_id = ?", orgID).Delete(&model.APIKey{})
	db.Where("org_id = ?", orgID).Delete(&model.AuditEntry{})
	db.Where("org_id = ?", orgID).Delete(&model.Token{})
	db.Where("org_id = ?", orgID).Delete(&model.Credential{})
	db.Where("id = ?", orgID).Delete(&model.Org{})
}

func createTestOrg(t *testing.T, db *gorm.DB) model.Org {
	t.Helper()
	org := model.Org{
		ID:         uuid.New(),
		Name:       fmt.Sprintf("apikey-handler-%s", uuid.New().String()[:8]),
		LogtoOrgID: fmt.Sprintf("logto-handler-%s", uuid.New().String()[:8]),
		RateLimit:  1000,
		Active:     true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, org.ID) })
	return org
}

type apiKeyTestHarness struct {
	db       *gorm.DB
	cache    *cache.APIKeyCache
	manager  *cache.Manager
	handler  *handler.APIKeyHandler
	router   *chi.Mux
}

func newAPIKeyHarness(t *testing.T) *apiKeyTestHarness {
	t.Helper()

	db := connectTestDB(t)
	rc := connectTestRedis(t)
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)
	cm := cache.Build(cache.Config{
		MemMaxSize: 100,
		MemTTL:     5 * time.Minute,
		RedisTTL:   10 * time.Minute,
		DEKMaxSize: 100,
		DEKTTL:     10 * time.Minute,
		HardExpiry: 15 * time.Minute,
	}, rc, nil, db, keyCache)

	h := handler.NewAPIKeyHandler(db, keyCache, cm)

	r := chi.NewRouter()
	r.Route("/v1/api-keys", func(r chi.Router) {
		r.Post("/", h.Create)
		r.Get("/", h.List)
		r.Delete("/{id}", h.Revoke)
	})

	return &apiKeyTestHarness{
		db:      db,
		cache:   keyCache,
		manager: cm,
		handler: h,
		router:  r,
	}
}

func (h *apiKeyTestHarness) doRequest(t *testing.T, method, path string, body any, org *model.Org) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if org != nil {
		req = middleware.WithOrg(req, org)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

// --------------------------------------------------------------------------
// POST /v1/api-keys — Create
// --------------------------------------------------------------------------

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

	// Plaintext key must be present and have correct prefix
	key, ok := resp["key"].(string)
	if !ok || !strings.HasPrefix(key, "llmv_sk_") {
		t.Fatalf("expected key with llmv_sk_ prefix, got %v", resp["key"])
	}
	if len(key) != 72 {
		t.Fatalf("expected key length 72, got %d", len(key))
	}

	// Key prefix must be present
	prefix, ok := resp["key_prefix"].(string)
	if !ok || len(prefix) != 16 {
		t.Fatalf("expected key_prefix length 16, got %v", resp["key_prefix"])
	}

	// Scopes
	scopes, ok := resp["scopes"].([]any)
	if !ok || len(scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %v", resp["scopes"])
	}

	// Name
	if resp["name"] != "prod-key" {
		t.Fatalf("expected name 'prod-key', got %v", resp["name"])
	}

	// Verify in DB
	var dbKey model.APIKey
	if err := h.db.Where("id = ?", resp["id"]).First(&dbKey).Error; err != nil {
		t.Fatalf("key not found in DB: %v", err)
	}
	if dbKey.OrgID != org.ID {
		t.Fatalf("expected org ID %s, got %s", org.ID, dbKey.OrgID)
	}

	// Hash should match
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

	// Should be approximately 30 days from now
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

// --------------------------------------------------------------------------
// GET /v1/api-keys — List
// --------------------------------------------------------------------------

func TestAPIKeyHandler_List_ReturnsKeys(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	// Create two keys
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

	// Verify plaintext key is NOT in the list response
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

	// Create key in org1
	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "org1-key",
		"scopes": []string{"all"},
	}, &org1)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", rr.Code)
	}

	// List from org2 — should NOT see org1's key
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
		// Small delay so created_at differs
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

	// Most recent first
	if page.Data[0]["name"] != "third" {
		t.Fatalf("expected first entry to be 'third' (newest), got %v", page.Data[0]["name"])
	}
}

func TestAPIKeyHandler_List_IncludesRevokedKeys(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	// Create and revoke a key
	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "will-revoke",
		"scopes": []string{"all"},
	}, &org)
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)

	h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+created["id"].(string), nil, &org)

	// List should include the revoked key
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

// --------------------------------------------------------------------------
// DELETE /v1/api-keys/{id} — Revoke
// --------------------------------------------------------------------------

func TestAPIKeyHandler_Revoke_Success(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	// Create
	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "to-revoke",
		"scopes": []string{"all"},
	}, &org)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", rr.Code)
	}
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)

	// Revoke
	rr = h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+created["id"].(string), nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["status"] != "revoked" {
		t.Fatalf("expected status 'revoked', got %q", body["status"])
	}

	// Verify in DB
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

	// Create
	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "cache-test",
		"scopes": []string{"all"},
	}, &org)
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)

	// Pre-populate cache
	keyHash := model.HashAPIKey(created["key"].(string))
	h.cache.Set(keyHash, &cache.CachedAPIKey{
		ID:     uuid.MustParse(created["id"].(string)),
		OrgID:  org.ID,
		Scopes: []string{"all"},
	})

	// Verify it's cached
	if _, ok := h.cache.Get(keyHash); !ok {
		t.Fatal("expected key to be cached before revoke")
	}

	// Revoke
	h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+created["id"].(string), nil, &org)

	// Cache should be invalidated
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

	// Create
	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "double-revoke",
		"scopes": []string{"all"},
	}, &org)
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)

	// Revoke once
	h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+created["id"].(string), nil, &org)

	// Revoke again — should get 404
	rr = h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+created["id"].(string), nil, &org)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for already-revoked key, got %d", rr.Code)
	}
}

func TestAPIKeyHandler_Revoke_WrongOrg(t *testing.T) {
	h := newAPIKeyHarness(t)
	org1 := createTestOrg(t, h.db)
	org2 := createTestOrg(t, h.db)

	// Create key in org1
	rr := h.doRequest(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "org1-secret",
		"scopes": []string{"all"},
	}, &org1)
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)

	// Try to revoke from org2
	rr = h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+created["id"].(string), nil, &org2)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (wrong org), got %d", rr.Code)
	}

	// Verify key is NOT revoked in DB
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

// --------------------------------------------------------------------------
// Full lifecycle: Create → List → Revoke → List
// --------------------------------------------------------------------------

func TestAPIKeyHandler_FullLifecycle(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	// 1. Create
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

	// 2. List — should include the key
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

	// 3. Verify the key can authenticate
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)
	var authedOrg *model.Org
	authHandler := middleware.APIKeyAuth(h.db, keyCache)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	// 4. Revoke
	rr = h.doRequest(t, http.MethodDelete, "/v1/api-keys/"+keyID, nil, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke: expected 200, got %d", rr.Code)
	}

	// 5. Verify the key can NO longer authenticate
	keyCache2 := cache.NewAPIKeyCache(100, 5*time.Minute)
	authHandler2 := middleware.APIKeyAuth(h.db, keyCache2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for revoked key")
	}))

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Authorization", "Bearer "+plaintext)
	authRR2 := httptest.NewRecorder()
	authHandler2.ServeHTTP(authRR2, req2)
	if authRR2.Code != http.StatusUnauthorized {
		t.Fatalf("auth after revoke: expected 401, got %d", authRR2.Code)
	}

	// 6. List — should still show the key (with revoked_at set)
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
