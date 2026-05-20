package middleware_test

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

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
// ResolveOrgFlexible skips JWT resolution when org is already set.
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
