package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

type usageHarness struct {
	db      *gorm.DB
	handler *handler.UsageHandler
	router  *chi.Mux
}

func newUsageHarness(t *testing.T) *usageHarness {
	t.Helper()
	db := connectTestDB(t)
	h := handler.NewUsageHandler(db)
	r := chi.NewRouter()
	r.Get("/v1/usage", func(w http.ResponseWriter, req *http.Request) {
		// Inject org into context
		h.Get(w, req)
	})
	return &usageHarness{db: db, handler: h, router: r}
}

func (h *usageHarness) doRequest(t *testing.T, org *model.Org) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	if org != nil {
		req = middleware.WithOrg(req, org)
	}
	rr := httptest.NewRecorder()
	// Use a router that injects the org
	r := chi.NewRouter()
	r.Get("/v1/usage", func(w http.ResponseWriter, r *http.Request) {
		if org != nil {
			r = middleware.WithOrg(r, org)
		}
		h.handler.Get(w, r)
	})
	r.ServeHTTP(rr, req)
	return rr
}

func TestUsageHandler_EmptyOrg(t *testing.T) {
	h := newUsageHarness(t)
	org := createTestOrg(t, h.db)
	t.Cleanup(func() { cleanupOrg(t, h.db, org.ID) })

	rr := h.doRequest(t, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Credentials struct {
			Total   int64 `json:"total"`
			Active  int64 `json:"active"`
			Revoked int64 `json:"revoked"`
		} `json:"credentials"`
		Tokens struct {
			Total int64 `json:"total"`
		} `json:"tokens"`
		APIKeys struct {
			Total int64 `json:"total"`
		} `json:"api_keys"`
		Requests struct {
			Total     int64 `json:"total"`
			Today     int64 `json:"today"`
			Yesterday int64 `json:"yesterday"`
			Last7d    int64 `json:"last_7d"`
			Last30d   int64 `json:"last_30d"`
		} `json:"requests"`
		DailyRequests  []any `json:"daily_requests"`
		TopCredentials []any `json:"top_credentials"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Credentials.Total != 0 {
		t.Errorf("credentials.total: got %d, want 0", resp.Credentials.Total)
	}
	if resp.Tokens.Total != 0 {
		t.Errorf("tokens.total: got %d, want 0", resp.Tokens.Total)
	}
	if resp.Requests.Total != 0 {
		t.Errorf("requests.total: got %d, want 0", resp.Requests.Total)
	}
	if resp.DailyRequests == nil {
		t.Error("daily_requests should be empty array, not null")
	}
	if resp.TopCredentials == nil {
		t.Error("top_credentials should be empty array, not null")
	}
}

func TestUsageHandler_WithData(t *testing.T) {
	h := newUsageHarness(t)
	org := createTestOrg(t, h.db)
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.AuditEntry{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Token{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.Credential{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.APIKey{})
		cleanupOrg(t, h.db, org.ID)
	})

	// Create credentials
	dummyKey := []byte("encrypted-test-key-placeholder-32")
	dummyDEK := []byte("wrapped-dek-placeholder")
	cred1 := model.Credential{
		ID:           uuid.New(),
		OrgID:        org.ID,
		Label:        "active-cred",
		BaseURL:      "https://api.openai.com/v1",
		ProviderID:   "openai",
		EncryptedKey: dummyKey,
		WrappedDEK:   dummyDEK,
	}
	cred2 := model.Credential{
		ID:           uuid.New(),
		OrgID:        org.ID,
		Label:        "revoked-cred",
		BaseURL:      "https://api.anthropic.com/v1",
		ProviderID:   "anthropic",
		EncryptedKey: dummyKey,
		WrappedDEK:   dummyDEK,
		RevokedAt:    ptrTime(time.Now()),
	}
	h.db.Create(&cred1)
	h.db.Create(&cred2)

	// Create tokens
	now := time.Now()
	tok1 := model.Token{
		ID:           uuid.New(),
		OrgID:        org.ID,
		CredentialID: cred1.ID,
		JTI:          fmt.Sprintf("jti-%s", uuid.New().String()[:8]),
		ExpiresAt:    now.Add(time.Hour),
	}
	tok2 := model.Token{
		ID:           uuid.New(),
		OrgID:        org.ID,
		CredentialID: cred1.ID,
		JTI:          fmt.Sprintf("jti-%s", uuid.New().String()[:8]),
		ExpiresAt:    now.Add(-time.Hour), // expired
	}
	tok3 := model.Token{
		ID:           uuid.New(),
		OrgID:        org.ID,
		CredentialID: cred1.ID,
		JTI:          fmt.Sprintf("jti-%s", uuid.New().String()[:8]),
		ExpiresAt:    now.Add(time.Hour),
		RevokedAt:    ptrTime(now),
	}
	h.db.Create(&tok1)
	h.db.Create(&tok2)
	h.db.Create(&tok3)


	// Create audit entries (proxy requests)
	for i := 0; i < 5; i++ {
		h.db.Create(&model.AuditEntry{
			OrgID:        org.ID,
			CredentialID: &cred1.ID,
			Action:       "proxy.request",
			Metadata:     model.JSON{"method": "POST", "path": "/v1/chat/completions", "status": 200},
		})
	}
	// Add yesterday's audit entries
	yesterday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1).Add(12 * time.Hour)
	for i := 0; i < 2; i++ {
		entry := model.AuditEntry{
			OrgID:        org.ID,
			CredentialID: &cred1.ID,
			Action:       "proxy.request",
			Metadata:     model.JSON{"method": "POST", "path": "/v1/chat/completions", "status": 200},
		}
		h.db.Create(&entry)
		h.db.Model(&entry).Update("created_at", yesterday)
	}
	// Add some older audit entries (8 days ago)
	for i := 0; i < 3; i++ {
		entry := model.AuditEntry{
			OrgID:        org.ID,
			CredentialID: &cred1.ID,
			Action:       "proxy.request",
			Metadata:     model.JSON{"method": "POST", "path": "/v1/chat/completions", "status": 200},
		}
		h.db.Create(&entry)
		h.db.Model(&entry).Update("created_at", now.AddDate(0, 0, -8))
	}

	rr := h.doRequest(t, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Credentials struct {
			Total   int64 `json:"total"`
			Active  int64 `json:"active"`
			Revoked int64 `json:"revoked"`
		} `json:"credentials"`
		Tokens struct {
			Total   int64 `json:"total"`
			Active  int64 `json:"active"`
			Expired int64 `json:"expired"`
			Revoked int64 `json:"revoked"`
		} `json:"tokens"`
		Requests struct {
			Total     int64 `json:"total"`
			Today     int64 `json:"today"`
			Yesterday int64 `json:"yesterday"`
			Last7d    int64 `json:"last_7d"`
			Last30d   int64 `json:"last_30d"`
		} `json:"requests"`
		TopCredentials []struct {
			ID           string `json:"id"`
			Label        string `json:"label"`
			ProviderID   string `json:"provider_id"`
			RequestCount int64  `json:"request_count"`
		} `json:"top_credentials"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Credentials
	if resp.Credentials.Total != 2 {
		t.Errorf("credentials.total: got %d, want 2", resp.Credentials.Total)
	}
	if resp.Credentials.Active != 1 {
		t.Errorf("credentials.active: got %d, want 1", resp.Credentials.Active)
	}
	if resp.Credentials.Revoked != 1 {
		t.Errorf("credentials.revoked: got %d, want 1", resp.Credentials.Revoked)
	}

	// Tokens
	if resp.Tokens.Total != 3 {
		t.Errorf("tokens.total: got %d, want 3", resp.Tokens.Total)
	}
	if resp.Tokens.Active != 1 {
		t.Errorf("tokens.active: got %d, want 1", resp.Tokens.Active)
	}
	if resp.Tokens.Expired != 1 {
		t.Errorf("tokens.expired: got %d, want 1", resp.Tokens.Expired)
	}
	if resp.Tokens.Revoked != 1 {
		t.Errorf("tokens.revoked: got %d, want 1", resp.Tokens.Revoked)
	}
	// Requests (5 today + 2 yesterday + 3 from 8 days ago = 10 total)
	if resp.Requests.Total != 10 {
		t.Errorf("requests.total: got %d, want 10", resp.Requests.Total)
	}
	if resp.Requests.Today != 5 {
		t.Errorf("requests.today: got %d, want 5", resp.Requests.Today)
	}
	if resp.Requests.Yesterday != 2 {
		t.Errorf("requests.yesterday: got %d, want 2", resp.Requests.Yesterday)
	}
	if resp.Requests.Last7d != 7 {
		t.Errorf("requests.last_7d: got %d, want 7", resp.Requests.Last7d)
	}
	if resp.Requests.Last30d != 10 {
		t.Errorf("requests.last_30d: got %d, want 10", resp.Requests.Last30d)
	}

	// Top credentials
	if len(resp.TopCredentials) != 1 {
		t.Fatalf("top_credentials: got %d, want 1", len(resp.TopCredentials))
	}
	if resp.TopCredentials[0].Label != "active-cred" {
		t.Errorf("top_credentials[0].label: got %q, want %q", resp.TopCredentials[0].Label, "active-cred")
	}
	if resp.TopCredentials[0].ProviderID != "openai" {
		t.Errorf("top_credentials[0].provider_id: got %q, want %q", resp.TopCredentials[0].ProviderID, "openai")
	}
}

func TestUsageHandler_NoOrgContext(t *testing.T) {
	h := newUsageHarness(t)
	rr := h.doRequest(t, nil)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestUsageHandler_OrgIsolation(t *testing.T) {
	h := newUsageHarness(t)
	org1 := createTestOrg(t, h.db)
	org2 := createTestOrg(t, h.db)
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org1.ID).Delete(&model.Credential{})
		h.db.Where("org_id = ?", org2.ID).Delete(&model.Credential{})
		cleanupOrg(t, h.db, org1.ID)
		cleanupOrg(t, h.db, org2.ID)
	})

	dummyKey := []byte("encrypted-test-key-placeholder-32")
	dummyDEK := []byte("wrapped-dek-placeholder")
	// Create credential in org1
	h.db.Create(&model.Credential{
		ID:           uuid.New(),
		OrgID:        org1.ID,
		Label:        "org1-cred",
		BaseURL:      "https://api.openai.com/v1",
		EncryptedKey: dummyKey,
		WrappedDEK:   dummyDEK,
	})
	// Create credential in org2
	h.db.Create(&model.Credential{
		ID:           uuid.New(),
		OrgID:        org2.ID,
		Label:        "org2-cred",
		BaseURL:      "https://api.openai.com/v1",
		EncryptedKey: dummyKey,
		WrappedDEK:   dummyDEK,
	})

	// Query as org1
	rr := h.doRequest(t, &org1)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		Credentials struct {
			Total int64 `json:"total"`
		} `json:"credentials"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.Credentials.Total != 1 {
		t.Errorf("org1 credentials.total: got %d, want 1 (should not see org2's data)", resp.Credentials.Total)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
