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

func TestIntegration_APIKeyAuth_RevokedKey(t *testing.T) {
	db := connectTestDB(t)
	keyCache := cache.NewAPIKeyCache(100, 5*time.Minute)

	orgID := uuid.New()
	org := model.Org{
		ID:   orgID,
		Name: fmt.Sprintf("apikey-revoked-%s", uuid.New().String()[:8]),

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
		ID:   orgID,
		Name: fmt.Sprintf("apikey-expired-%s", uuid.New().String()[:8]),

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
		ID:   orgID,
		Name: fmt.Sprintf("apikey-inactive-%s", uuid.New().String()[:8]),

		RateLimit: 1000,
		Active:    true,
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
