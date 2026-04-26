package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

const (
	testMinioEndpoint = "http://localhost:9000"
	testMinioAccess   = "minioadmin"
	testMinioSecret   = "minioadmin"
	testMinioBucket   = "public-files-test"
)

func newRealPresigner(t *testing.T) *storage.S3Presigner {
	t.Helper()
	endpoint := os.Getenv("PUBLIC_ASSETS_S3_ENDPOINT")
	if endpoint == "" {
		endpoint = testMinioEndpoint
	}
	hcReq, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint+"/minio/health/ready", nil)
	if resp, err := http.DefaultClient.Do(hcReq); err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Skipf("MinIO not reachable at %s: %v", endpoint, err)
	} else {
		_ = resp.Body.Close()
	}
	p, err := storage.NewS3Presigner(storage.PublicAssetsConfig{
		Bucket:     testMinioBucket,
		Region:     "auto",
		Endpoint:   endpoint,
		AccessKey:  testMinioAccess,
		SecretKey:  testMinioSecret,
		PublicBase: endpoint + "/" + testMinioBucket,
		SignTTL:    15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("create presigner: %v", err)
	}
	return p
}

type uploadsTestHarness struct {
	db      *gorm.DB
	handler *handler.UploadsHandler
	router  *chi.Mux
}

func newUploadsHarness(t *testing.T) *uploadsTestHarness {
	t.Helper()
	db := connectTestDB(t)
	h := handler.NewUploadsHandler(db, newRealPresigner(t))
	r := chi.NewRouter()
	r.Post("/v1/uploads/sign", h.Sign)
	return &uploadsTestHarness{db: db, handler: h, router: r}
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
	_ = json.NewDecoder(rr.Body).Decode(&resp)
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

func TestUploadsSign_OrgLogo_AnyMemberAllowed(t *testing.T) {
	h := newUploadsHarness(t)
	member := createTestUser(t, h.db, fmt.Sprintf("upmemb-%s@test.com", uuid.New().String()[:8]))
	org := createTestOrg(t, h.db)
	addOrgMembership(t, h.db, member.ID, org.ID, "member")

	rr := h.doSign(t, map[string]any{
		"asset_type":   "org_logo",
		"content_type": "image/png",
		"size_bytes":   2048,
	}, &member, &org)
	if rr.Code != http.StatusOK {
		t.Fatalf("plain member: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	key := resp["key"].(string)
	want := "pub/o/" + org.ID.String() + "/"
	if !bytesHasPrefix(key, want) {
		t.Fatalf("expected key prefix %q, got %q", want, key)
	}

	outsider := createTestUser(t, h.db, fmt.Sprintf("upout-%s@test.com", uuid.New().String()[:8]))
	rr = h.doSign(t, map[string]any{
		"asset_type":   "org_logo",
		"content_type": "image/png",
		"size_bytes":   2048,
	}, &outsider, &org)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("non-member: expected 403, got %d: %s", rr.Code, rr.Body.String())
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
