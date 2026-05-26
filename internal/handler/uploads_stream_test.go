package handler_test

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
)

type streamHarness struct {
	db            *gorm.DB
	router        *chi.Mux
	orgID         uuid.UUID
	convID        uuid.UUID
	sandboxID     uuid.UUID
	runtimeSecret string
	publicBase    string
	publicAsset   *handler.UploadsHandler
}

func newStreamHarness(t *testing.T) *streamHarness {
	t.Helper()
	db := connectTestDB(t)
	presigner := newRealPresigner(t)
	encKey := testSymmetricKey(t)

	h := handler.NewUploadsHandler(db, presigner)
	h.WithStreamer(presigner, encKey)

	r := chi.NewRouter()
	r.Put("/internal/conversations/{conversationID}/assets/*", h.StreamConversationAsset)
	r.Post("/internal/conversations/{conversationID}/assets/move", h.MoveConversationAsset)
	r.Delete("/internal/conversations/{conversationID}/assets/*", h.DeleteConversationAsset)

	orgID := uuid.New()
	if err := db.Create(&model.Org{
		ID:        orgID,
		Name:      fmt.Sprintf("stream-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", orgID).Delete(&model.Org{}) })

	agentID := uuid.New()
	if err := db.Create(&model.Employee{
		ID:     agentID,
		OrgID:  &orgID,
		Name:   "stream-agent",
		Status: "active",
	}).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	runtimeSecret := fmt.Sprintf("test-runtime-key-%s", uuid.New().String()[:8])
	encrypted, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}

	sandboxID := uuid.New()
	if err := db.Create(&model.Sandbox{
		ID:                     sandboxID,
		OrgID:                  &orgID,
		EmployeeID:             &agentID,
		EncryptedRuntimeSecret: encrypted,
		Status:                 "running",
		ExternalID:             "mock-external-id",
		RuntimeURL:             "http://localhost:25434",
	}).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	convID := uuid.New()
	if err := db.Create(&model.EmployeeSession{
		ID:                    convID,
		OrgID:                 orgID,
		EmployeeID:            agentID,
		SandboxID:             sandboxID,
		RuntimeConversationID: "runtime-conv-" + uuid.New().String()[:8],
		Status:                "active",
	}).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	return &streamHarness{
		db:            db,
		router:        r,
		orgID:         orgID,
		convID:        convID,
		sandboxID:     sandboxID,
		runtimeSecret: runtimeSecret,
		publicBase:    testMinioEndpoint + "/" + testMinioBucket,
		publicAsset:   h,
	}
}

func (s *streamHarness) put(t *testing.T, urlPath string, body io.Reader, contentType, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, urlPath, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)
	return rr
}

func TestStreamAsset_HappyPath_Image(t *testing.T) {
	h := newStreamHarness(t)
	body := []byte("\x89PNG\r\n\x1a\nfake-bytes")

	rr := h.put(t,
		fmt.Sprintf("/internal/conversations/%s/assets/images/cat.png", h.convID),
		bytes.NewReader(body),
		"image/png",
		h.runtimeSecret,
	)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID          string `json:"id"`
		PublicURL   string `json:"asset_url"`
		Key         string `json:"key"`
		Path        string `json:"path"`
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Bytes       int64  `json:"bytes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Path != "images" || resp.Filename != "cat.png" {
		t.Fatalf("path/filename mismatch: %+v", resp)
	}
	if resp.ContentType != "image/png" {
		t.Fatalf("content type: got %q", resp.ContentType)
	}
	if resp.Bytes != int64(len(body)) {
		t.Fatalf("bytes: got %d want %d", resp.Bytes, len(body))
	}
	wantKey := fmt.Sprintf("pub/c/%s/images/cat.png", h.convID)
	if resp.Key != wantKey {
		t.Fatalf("key: got %q want %q", resp.Key, wantKey)
	}

	// Verify the object is fetchable via the asset URL.
	getReq, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, resp.PublicURL, nil)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("get asset URL: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("public GET: got %d", getResp.StatusCode)
	}
	got, _ := io.ReadAll(getResp.Body)
	if !bytes.Equal(got, body) {
		t.Fatalf("public bytes mismatch")
	}

	// And the row was persisted.
	var row model.ConversationAsset
	if err := h.db.Where("key = ?", wantKey).First(&row).Error; err != nil {
		t.Fatalf("load asset row: %v", err)
	}
	if row.ConversationID != h.convID || row.OrgID != h.orgID || row.SandboxID != h.sandboxID {
		t.Fatalf("row scoping wrong: %+v", row)
	}
}

func TestStreamAsset_LargeMultipartStream(t *testing.T) {
	h := newStreamHarness(t)

	// 24MB random body forces the SDK uploader to use multipart (8MB parts).
	const size = 24 * 1024 * 1024
	body := make([]byte, size)
	if _, err := rand.Read(body); err != nil {
		t.Fatalf("rand: %v", err)
	}

	rr := h.put(t,
		fmt.Sprintf("/internal/conversations/%s/assets/videos/big.bin", h.convID),
		bytes.NewReader(body),
		"application/octet-stream",
		h.runtimeSecret,
	)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Bytes int64 `json:"bytes"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Bytes != int64(size) {
		t.Fatalf("bytes: got %d want %d", resp.Bytes, size)
	}
}

func TestStreamAsset_OverwriteByPath(t *testing.T) {
	h := newStreamHarness(t)
	urlPath := fmt.Sprintf("/internal/conversations/%s/assets/exports/data.csv", h.convID)

	first := h.put(t, urlPath, bytes.NewReader([]byte("v1,a")), "text/csv", h.runtimeSecret)
	if first.Code != http.StatusCreated {
		t.Fatalf("first upload: got %d: %s", first.Code, first.Body.String())
	}
	var firstResp struct {
		ID    string `json:"id"`
		Bytes int64  `json:"bytes"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &firstResp)

	second := h.put(t, urlPath, bytes.NewReader([]byte("v2,a-longer-second-version")), "text/csv", h.runtimeSecret)
	if second.Code != http.StatusCreated {
		t.Fatalf("second upload: got %d: %s", second.Code, second.Body.String())
	}
	var secondResp struct {
		ID    string `json:"id"`
		Bytes int64  `json:"bytes"`
	}
	_ = json.Unmarshal(second.Body.Bytes(), &secondResp)

	if secondResp.ID != firstResp.ID {
		t.Fatalf("expected same row id (overwrite); got first=%s second=%s", firstResp.ID, secondResp.ID)
	}
	if secondResp.Bytes == firstResp.Bytes {
		t.Fatalf("expected new byte count after overwrite")
	}

	// Confirm the bucket reflects v2.
	wantKey := fmt.Sprintf("pub/c/%s/exports/data.csv", h.convID)
	var row model.ConversationAsset
	if err := h.db.Where("key = ?", wantKey).First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if row.Bytes != secondResp.Bytes {
		t.Fatalf("row bytes %d != response bytes %d", row.Bytes, secondResp.Bytes)
	}

	// Check exactly one row exists for this key.
	var count int64
	h.db.Model(&model.ConversationAsset{}).Where("key = ?", wantKey).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row for key, got %d", count)
	}
}

func TestStreamAsset_RootFolder(t *testing.T) {
	h := newStreamHarness(t)
	rr := h.put(t,
		fmt.Sprintf("/internal/conversations/%s/assets/loose.txt", h.convID),
		bytes.NewReader([]byte("hello")),
		"text/plain",
		h.runtimeSecret,
	)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Path     string `json:"path"`
		Filename string `json:"filename"`
		Key      string `json:"key"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Path != "" || resp.Filename != "loose.txt" {
		t.Fatalf("expected root file: %+v", resp)
	}
	wantKey := fmt.Sprintf("pub/c/%s/loose.txt", h.convID)
	if resp.Key != wantKey {
		t.Fatalf("key: got %q want %q", resp.Key, wantKey)
	}
}
