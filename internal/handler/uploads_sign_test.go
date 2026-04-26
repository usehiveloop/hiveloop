package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/storage"
)

type fakePresigner struct {
	mu       sync.Mutex
	calls    []storage.SignRequest
	policies map[storage.AssetType]storage.AssetPolicy
}

func newFakePresigner() *fakePresigner {
	imageTypes := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp", "image/gif": "gif"}
	genericTypes := map[string]string{"image/png": "png", "application/pdf": "pdf", "text/plain": "txt"}
	return &fakePresigner{
		policies: map[storage.AssetType]storage.AssetPolicy{
			storage.AssetTypeAvatar: {
				MaxBytes:     5 * 1024 * 1024,
				AllowedTypes: imageTypes,
				KeyPrefix: func(r storage.SignRequest) (string, error) {
					return fmt.Sprintf("avatars/%s/", r.UserID), nil
				},
			},
			storage.AssetTypeOrgLogo: {
				MaxBytes:     5 * 1024 * 1024,
				AllowedTypes: imageTypes,
				KeyPrefix: func(r storage.SignRequest) (string, error) {
					if r.OrgID == nil {
						return "", fmt.Errorf("org_id required")
					}
					return fmt.Sprintf("pub/o/%s/", *r.OrgID), nil
				},
			},
			storage.AssetTypeGeneric: {
				MaxBytes:     25 * 1024 * 1024,
				AllowedTypes: genericTypes,
				KeyPrefix: func(r storage.SignRequest) (string, error) {
					return fmt.Sprintf("pub/u/%s/", r.UserID), nil
				},
			},
		},
	}
}

func (f *fakePresigner) Policy(t storage.AssetType) (storage.AssetPolicy, bool) {
	p, ok := f.policies[t]
	return p, ok
}

func (f *fakePresigner) Sign(_ context.Context, req storage.SignRequest) (*storage.SignedUpload, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.mu.Unlock()
	pol := f.policies[req.AssetType]
	prefix, err := pol.KeyPrefix(req)
	if err != nil {
		return nil, err
	}
	key := prefix + uuid.New().String() + ".bin"
	return &storage.SignedUpload{
		UploadURL:       "https://example.com/" + key + "?sig=test",
		UploadMethod:    "PUT",
		RequiredHeaders: map[string]string{"Content-Type": req.ContentType},
		Key:             key,
		PublicURL:       "https://public.example.com/" + key,
		ExpiresAt:       time.Now().Add(15 * time.Minute).UTC(),
		MaxSizeBytes:    pol.MaxBytes,
	}, nil
}

type uploadsTestHarness struct {
	db        *gorm.DB
	presigner *fakePresigner
	handler   *handler.UploadsHandler
	router    *chi.Mux
}

func newUploadsHarness(t *testing.T) *uploadsTestHarness {
	t.Helper()
	db := connectTestDB(t)
	fp := newFakePresigner()
	h := handler.NewUploadsHandler(db, fp)
	r := chi.NewRouter()
	r.Post("/v1/uploads/sign", h.Sign)
	return &uploadsTestHarness{db: db, presigner: fp, handler: h, router: r}
}

func (h *uploadsTestHarness) doSign(t *testing.T, body any, user *model.User, org *model.Org) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/uploads/sign", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if user != nil {
		req = middleware.WithUser(req, user)
	}
	if org != nil {
		req = middleware.WithOrg(req, org)
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func addOrgMembership(t *testing.T, db *gorm.DB, userID, orgID uuid.UUID, role string) {
	t.Helper()
	m := model.OrgMembership{
		ID:     uuid.New(),
		UserID: userID,
		OrgID:  orgID,
		Role:   role,
	}
	if err := db.Create(&m).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", m.ID).Delete(&model.OrgMembership{}) })
}

func TestUploadsSign_Avatar_HappyPath(t *testing.T) {
	h := newUploadsHarness(t)
	user := createTestUser(t, h.db, fmt.Sprintf("uploads-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, h.db)

	rr := h.doSign(t, map[string]any{
		"asset_type":   "avatar",
		"content_type": "image/png",
		"size_bytes":   1024,
		"filename":     "me.png",
	}, &user, &org)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["upload_method"] != "PUT" {
		t.Fatalf("expected upload_method=PUT, got %v", resp["upload_method"])
	}
	if resp["upload_url"] == "" || resp["public_url"] == "" || resp["key"] == "" {
		t.Fatalf("missing fields: %#v", resp)
	}
	key := resp["key"].(string)
	if !bytesHasPrefix(key, "avatars/"+user.ID.String()+"/") {
		t.Fatalf("expected avatars/<user>/ key prefix, got %q", key)
	}
	if int64(resp["max_size_bytes"].(float64)) != 5*1024*1024 {
		t.Fatalf("unexpected max_size_bytes: %v", resp["max_size_bytes"])
	}
}

func TestUploadsSign_OrgLogo_RequiresAdmin(t *testing.T) {
	h := newUploadsHarness(t)
	user := createTestUser(t, h.db, fmt.Sprintf("upmemb-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, h.db)
	addOrgMembership(t, h.db, user.ID, org.ID, "member")

	rr := h.doSign(t, map[string]any{
		"asset_type":   "org_logo",
		"content_type": "image/png",
		"size_bytes":   2048,
	}, &user, &org)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("non-admin: expected 403, got %d: %s", rr.Code, rr.Body.String())
	}

	admin := createTestUser(t, h.db, fmt.Sprintf("upadmin-%s@test.com", uuid.New().String()[:8]))
	addOrgMembership(t, h.db, admin.ID, org.ID, "admin")
	rr = h.doSign(t, map[string]any{
		"asset_type":   "org_logo",
		"content_type": "image/png",
		"size_bytes":   2048,
	}, &admin, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	key := resp["key"].(string)
	want := "pub/o/" + org.ID.String() + "/"
	if !bytesHasPrefix(key, want) {
		t.Fatalf("expected key prefix %q, got %q", want, key)
	}
}

func TestUploadsSign_RejectsUnknownAssetType(t *testing.T) {
	h := newUploadsHarness(t)
	user := createTestUser(t, h.db, fmt.Sprintf("upun-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, h.db)

	rr := h.doSign(t, map[string]any{
		"asset_type":   "weird",
		"content_type": "image/png",
		"size_bytes":   1024,
	}, &user, &org)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUploadsSign_RejectsContentTypeOutsideAllowlist(t *testing.T) {
	h := newUploadsHarness(t)
	user := createTestUser(t, h.db, fmt.Sprintf("upct-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, h.db)

	rr := h.doSign(t, map[string]any{
		"asset_type":   "avatar",
		"content_type": "application/pdf",
		"size_bytes":   1024,
	}, &user, &org)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUploadsSign_RejectsOversizeRequest(t *testing.T) {
	h := newUploadsHarness(t)
	user := createTestUser(t, h.db, fmt.Sprintf("upsz-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, h.db)

	rr := h.doSign(t, map[string]any{
		"asset_type":   "avatar",
		"content_type": "image/png",
		"size_bytes":   20 * 1024 * 1024,
	}, &user, &org)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUploadsSign_RequiresAuthentication(t *testing.T) {
	h := newUploadsHarness(t)
	rr := h.doSign(t, map[string]any{
		"asset_type":   "avatar",
		"content_type": "image/png",
		"size_bytes":   1024,
	}, nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func bytesHasPrefix(s, prefix string) bool { return len(s) >= len(prefix) && s[:len(prefix)] == prefix }
