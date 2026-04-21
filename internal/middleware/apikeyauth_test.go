package middleware_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/cache"
	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// --------------------------------------------------------------------------
// APIKeyAuth middleware — real Postgres
// --------------------------------------------------------------------------

func TestIntegration_APIKeyAuth_ValidKey(t *testing.T) {
	db := connectTestDB(t)
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)

	orgID := uuid.New()
	org := model.Org{
		ID:         orgID,
		Name:       fmt.Sprintf("apikey-valid-%s", uuid.New().String()[:8]),

		RateLimit:  1000,
		Active:     true,
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
		ID:         orgID,
		Name:       fmt.Sprintf("apikey-cache-%s", uuid.New().String()[:8]),

		RateLimit:  1000,
		Active:     true,
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

	// First request — cache miss, DB lookup
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

	// Second request — cache hit
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

func TestIntegration_APIKeyAuth_RevokedKey(t *testing.T) {
	db := connectTestDB(t)
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)

	orgID := uuid.New()
	org := model.Org{
		ID:         orgID,
		Name:       fmt.Sprintf("apikey-revoked-%s", uuid.New().String()[:8]),

		RateLimit:  1000,
		Active:     true,
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
	now := time.Now()
	apiKey := model.APIKey{
		ID:        uuid.New(),
		OrgID:     orgID,
		Name:      "revoked-key",
		KeyHash:   hash,
		KeyPrefix: prefix,
		Scopes:    []string{"all"},
		RevokedAt: &now,
	}
	if err := db.Create(&apiKey).Error; err != nil {
		t.Fatalf("failed to create api key: %v", err)
	}

	handler := middleware.APIKeyAuth(db, keyCache, &enqueue.MockClient{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for revoked key")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestIntegration_APIKeyAuth_ExpiredKey(t *testing.T) {
	db := connectTestDB(t)
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)

	orgID := uuid.New()
	org := model.Org{
		ID:         orgID,
		Name:       fmt.Sprintf("apikey-expired-%s", uuid.New().String()[:8]),

		RateLimit:  1000,
		Active:     true,
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
	expired := time.Now().Add(-time.Hour)
	apiKey := model.APIKey{
		ID:        uuid.New(),
		OrgID:     orgID,
		Name:      "expired-key",
		KeyHash:   hash,
		KeyPrefix: prefix,
		Scopes:    []string{"all"},
		ExpiresAt: &expired,
	}
	if err := db.Create(&apiKey).Error; err != nil {
		t.Fatalf("failed to create api key: %v", err)
	}

	handler := middleware.APIKeyAuth(db, keyCache, &enqueue.MockClient{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for expired key")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestIntegration_APIKeyAuth_InactiveOrg(t *testing.T) {
	db := connectTestDB(t)
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)

	orgID := uuid.New()
	org := model.Org{
		ID:         orgID,
		Name:       fmt.Sprintf("apikey-inactive-%s", uuid.New().String()[:8]),

		RateLimit:  1000,
		Active:     true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	// Deactivate the org (avoid GORM zero-value default issue)
	if err := db.Model(&org).Update("active", false).Error; err != nil {
		t.Fatalf("failed to deactivate org: %v", err)
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
		Name:      "inactive-org-key",
		KeyHash:   hash,
		KeyPrefix: prefix,
		Scopes:    []string{"all"},
	}
	if err := db.Create(&apiKey).Error; err != nil {
		t.Fatalf("failed to create api key: %v", err)
	}

	handler := middleware.APIKeyAuth(db, keyCache, &enqueue.MockClient{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for inactive org")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// --------------------------------------------------------------------------
// RequireAPIKeyScopeOrJWT — scope enforcement
// --------------------------------------------------------------------------

func TestRequireAPIKeyScopeOrJWT_AllowsMatchingScope(t *testing.T) {
	mw := middleware.RequireAPIKeyScopeOrJWT("credentials")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{
		KeyID:  "key-id",
		OrgID:  "org-id",
		Scopes: []string{"credentials"},
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRequireAPIKeyScopeOrJWT_AllScopeGrantsAccess(t *testing.T) {
	mw := middleware.RequireAPIKeyScopeOrJWT("credentials")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{
		KeyID:  "key-id",
		OrgID:  "org-id",
		Scopes: []string{"all"},
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (all scope grants access), got %d", rr.Code)
	}
}

func TestRequireAPIKeyScopeOrJWT_DeniesWrongScope(t *testing.T) {
	mw := middleware.RequireAPIKeyScopeOrJWT("credentials")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called with wrong scope")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{
		KeyID:  "key-id",
		OrgID:  "org-id",
		Scopes: []string{"connect"},
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["error"] != "api key lacks required scope: credentials" {
		t.Fatalf("unexpected error: %s", body["error"])
	}
}

func TestRequireAPIKeyScopeOrJWT_DeniesNoClaims(t *testing.T) {
	mw := middleware.RequireAPIKeyScopeOrJWT("credentials")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called without claims")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestRequireAPIKeyScopeOrJWT_MultipleScopes(t *testing.T) {
	mw := middleware.RequireAPIKeyScopeOrJWT("tokens")

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{
		KeyID:  "key-id",
		OrgID:  "org-id",
		Scopes: []string{"connect", "tokens"},
	})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (tokens in scopes list), got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// MultiAuth — dispatching based on token prefix
// --------------------------------------------------------------------------

func TestIntegration_MultiAuth_APIKeyPath(t *testing.T) {
	db := connectTestDB(t)
	// Generate a dummy RSA key -- the API key path never validates JWTs
	dummyKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)

	orgID := uuid.New()
	org := model.Org{
		ID:        orgID,
		Name:      fmt.Sprintf("multiauth-apikey-%s", uuid.New().String()[:8]),
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
		Name:      "multi-auth-key",
		KeyHash:   hash,
		KeyPrefix: prefix,
		Scopes:    []string{"all"},
	}
	if err := db.Create(&apiKey).Error; err != nil {
		t.Fatalf("failed to create api key: %v", err)
	}

	var gotClaims *middleware.APIKeyClaims
	handler := middleware.MultiAuth(&dummyKey.PublicKey, "test-issuer", "test-audience", db, keyCache, &enqueue.MockClient{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		gotClaims, ok = middleware.APIKeyClaimsFromContext(r.Context())
		if !ok {
			t.Fatal("api key claims not found via MultiAuth")
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
	if gotClaims.KeyID != apiKey.ID.String() {
		t.Fatalf("expected key ID %s, got %s", apiKey.ID, gotClaims.KeyID)
	}
}

func TestIntegration_MultiAuth_JWTPath(t *testing.T) {
	db := connectTestDB(t)
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)

	const testIssuer = "test-issuer"
	const testAudience = "test-audience"

	orgID := uuid.New()
	org := model.Org{
		ID:        orgID,
		Name:      fmt.Sprintf("multiauth-jwt-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	t.Cleanup(func() { cleanupOrg(t, db, orgID) })

	userID := uuid.New()
	user := model.User{ID: userID, Email: fmt.Sprintf("multiauth-%s@test.com", userID.String()[:8]), Name: "Test User"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	membership := model.OrgMembership{UserID: userID, OrgID: orgID, Role: "admin"}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("failed to create membership: %v", err)
	}

	jwtToken, err := auth.IssueAccessToken(privKey, testIssuer, testAudience, userID.String(), orgID.String(), "admin", time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	var gotOrg *model.Org
	handler := middleware.MultiAuth(&privKey.PublicKey, testIssuer, testAudience, db, keyCache, &enqueue.MockClient{})(
		middleware.ResolveOrgFlexible(db)(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var ok bool
				gotOrg, ok = middleware.OrgFromContext(r.Context())
				if !ok {
					t.Fatal("org not found via MultiAuth + JWT path")
				}
				w.WriteHeader(http.StatusOK)
			}),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/credentials", nil)
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("X-Org-ID", orgID.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if gotOrg == nil {
		t.Fatal("expected org to be resolved via JWT path")
	}
	if gotOrg.ID != orgID {
		t.Fatalf("expected org ID %s, got %s", orgID, gotOrg.ID)
	}
}

func TestIntegration_MultiAuth_MissingAuth(t *testing.T) {
	db := connectTestDB(t)
	dummyKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)

	handler := middleware.MultiAuth(&dummyKey.PublicKey, "test-issuer", "test-audience", db, keyCache, &enqueue.MockClient{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

// --------------------------------------------------------------------------
// ResolveOrgFlexible — skips JWT resolution when org already set
// --------------------------------------------------------------------------

func TestResolveOrgFlexible_SkipsWhenOrgSet(t *testing.T) {
	db := connectTestDB(t)

	org := &model.Org{ID: uuid.New(), Name: "test-org"}

	var gotOrg *model.Org
	handler := middleware.ResolveOrgFlexible(db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		gotOrg, ok = middleware.OrgFromContext(r.Context())
		if !ok {
			t.Fatal("org not found")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = middleware.WithOrg(req, org)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotOrg.ID != org.ID {
		t.Fatalf("expected org ID %s, got %s", org.ID, gotOrg.ID)
	}
}
