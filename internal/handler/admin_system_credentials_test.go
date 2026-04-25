package handler_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// These tests exercise the admin HTTP surface against a real Postgres and a
// real AEAD KMS wrapper. They reuse testDBURL + connectTestDB from the
// sibling api_keys_test.go so we don't duplicate scaffolding.

func newTestKMS(t *testing.T) *crypto.KeyWrapper {
	t.Helper()
	key := make([]byte, 32)
	b64 := base64.StdEncoding.EncodeToString(key)
	kms, err := crypto.NewAEADWrapper(context.Background(), b64, "test-admin-kms")
	if err != nil {
		t.Fatalf("KMS: %v", err)
	}
	return kms
}

func seedPlatformOrgForAdminTest(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := credentials.SeedPlatformOrg(db); err != nil {
		t.Fatalf("seed platform org: %v", err)
	}
}

func TestIntegration_AdminSystemCreds_Create_RejectsEmptyAPIKey(t *testing.T) {
	db := connectTestDB(t)
	seedPlatformOrgForAdminTest(t, db)

	h := handler.NewAdminSystemCredentialsHandler(db, newTestKMS(t), nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/system-credentials",
		bytes.NewBufferString(`{"provider_id":"moonshotai","api_key":""}`))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestIntegration_AdminSystemCreds_Create_RejectsUnknownProvider(t *testing.T) {
	db := connectTestDB(t)
	seedPlatformOrgForAdminTest(t, db)

	h := handler.NewAdminSystemCredentialsHandler(db, newTestKMS(t), nil)
	body := `{"provider_id":"bogus-provider","api_key":"sk-test"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/system-credentials", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestIntegration_AdminSystemCreds_CreateListRevoke_HappyPath(t *testing.T) {
	db := connectTestDB(t)
	seedPlatformOrgForAdminTest(t, db)
	h := handler.NewAdminSystemCredentialsHandler(db, newTestKMS(t), nil)

	// Create.
	body := `{"provider_id":"moonshotai","api_key":"sk-test-key","label":"test-system"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/system-credentials", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: status %d, want 201; body=%s", w.Code, w.Body.String())
	}
	var created struct {
		ID         string `json:"id"`
		Label      string `json:"label"`
		ProviderID string `json:"provider_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == "" || created.Label != "test-system" || created.ProviderID != "moonshotai" {
		t.Fatalf("unexpected create response: %+v", created)
	}
	t.Cleanup(func() { db.Unscoped().Where("id = ?", created.ID).Delete(&model.Credential{}) })

	// Verify the row in DB has is_system=true and is FK'd to the platform org.
	var row model.Credential
	if err := db.Where("id = ?", created.ID).First(&row).Error; err != nil {
		t.Fatalf("fetch created row: %v", err)
	}
	if !row.IsSystem {
		t.Errorf("row.IsSystem = false, want true")
	}
	if row.OrgID != credentials.PlatformOrgID {
		t.Errorf("row.OrgID = %s, want platform org %s", row.OrgID, credentials.PlatformOrgID)
	}
	if len(row.EncryptedKey) == 0 || len(row.WrappedDEK) == 0 {
		t.Errorf("encrypted_key or wrapped_dek empty; encryption didn't happen")
	}

	// List — the new cred should appear.
	listReq := httptest.NewRequest(http.MethodGet, "/admin/v1/system-credentials", nil)
	listW := httptest.NewRecorder()
	h.List(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list: status %d, want 200", listW.Code)
	}
	var listed []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(listW.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	found := false
	for _, c := range listed {
		if c.ID == created.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created credential %s not in list", created.ID)
	}

	// Revoke.
	revReq := httptest.NewRequest(http.MethodPost, "/admin/v1/system-credentials/"+created.ID+"/revoke", nil)
	// chi URL params are set via chi router in real routing; bypass by
	// calling the handler with a chi context.
	revReq = withChiURLParam(revReq, "id", created.ID)
	revW := httptest.NewRecorder()
	h.Revoke(revW, revReq)
	if revW.Code != http.StatusOK {
		t.Fatalf("revoke: status %d, want 200; body=%s", revW.Code, revW.Body.String())
	}

	// Verify revoked_at set.
	var afterRevoke model.Credential
	if err := db.Where("id = ?", created.ID).First(&afterRevoke).Error; err != nil {
		t.Fatalf("fetch after revoke: %v", err)
	}
	if afterRevoke.RevokedAt == nil {
		t.Errorf("revoked_at not set after revoke")
	}

	// Second revoke should 404.
	rev2 := httptest.NewRecorder()
	h.Revoke(rev2, withChiURLParam(
		httptest.NewRequest(http.MethodPost, "/admin/v1/system-credentials/"+created.ID+"/revoke", nil),
		"id", created.ID,
	))
	if rev2.Code != http.StatusNotFound {
		t.Errorf("second revoke: status %d, want 404 (already revoked)", rev2.Code)
	}
}

func TestIntegration_AdminSystemCreds_Revoke_RejectsNonSystemCred(t *testing.T) {
	db := connectTestDB(t)
	seedPlatformOrgForAdminTest(t, db)

	// Create a normal (non-system) credential — the admin revoke should NOT
	// touch it; its is_system = ? filter should exclude it.
	org := model.Org{Name: "byok-" + randSuffix()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("seed org: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&org) })

	userCred := model.Credential{
		OrgID:        org.ID,
		ProviderID:   "moonshotai",
		BaseURL:      "https://api.example.com",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("x"),
		WrappedDEK:   []byte("y"),
		IsSystem:     false,
	}
	if err := db.Create(&userCred).Error; err != nil {
		t.Fatalf("seed user cred: %v", err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&userCred) })

	h := handler.NewAdminSystemCredentialsHandler(db, newTestKMS(t), nil)
	req := withChiURLParam(
		httptest.NewRequest(http.MethodPost, "/admin/v1/system-credentials/"+userCred.ID.String()+"/revoke", nil),
		"id", userCred.ID.String(),
	)
	w := httptest.NewRecorder()
	h.Revoke(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("revoke on non-system cred: status %d, want 404", w.Code)
	}
	// And the cred is still active.
	var after model.Credential
	if err := db.Where("id = ?", userCred.ID).First(&after).Error; err != nil {
		t.Fatalf("fetch user cred: %v", err)
	}
	if after.RevokedAt != nil {
		t.Errorf("user cred was revoked — admin endpoint leaked onto non-system credentials")
	}
}
