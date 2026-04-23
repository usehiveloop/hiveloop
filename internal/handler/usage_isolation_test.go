package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

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
	h.db.Create(&model.Credential{
		ID:           uuid.New(),
		OrgID:        org1.ID,
		Label:        "org1-cred",
		BaseURL:      "https://api.openai.com/v1",
		EncryptedKey: dummyKey,
		WrappedDEK:   dummyDEK,
	})
	h.db.Create(&model.Credential{
		ID:           uuid.New(),
		OrgID:        org2.ID,
		Label:        "org2-cred",
		BaseURL:      "https://api.openai.com/v1",
		EncryptedKey: dummyKey,
		WrappedDEK:   dummyDEK,
	})

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
