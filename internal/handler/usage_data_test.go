package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

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
		ExpiresAt:    now.Add(-time.Hour),
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

	for i := 0; i < 5; i++ {
		h.db.Create(&model.AuditEntry{
			OrgID:        org.ID,
			CredentialID: &cred1.ID,
			Action:       "proxy.request",
			Metadata:     model.JSON{"method": "POST", "path": "/v1/chat/completions", "status": 200},
		})
	}
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

	if resp.Credentials.Total != 2 {
		t.Errorf("credentials.total: got %d, want 2", resp.Credentials.Total)
	}
	if resp.Credentials.Active != 1 {
		t.Errorf("credentials.active: got %d, want 1", resp.Credentials.Active)
	}
	if resp.Credentials.Revoked != 1 {
		t.Errorf("credentials.revoked: got %d, want 1", resp.Credentials.Revoked)
	}
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
