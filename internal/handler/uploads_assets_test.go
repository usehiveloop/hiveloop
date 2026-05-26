package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func (s *streamHarness) post(t *testing.T, urlPath, bodyJSON, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, urlPath, strings.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)
	return rr
}

func (s *streamHarness) delete(t *testing.T, urlPath, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, urlPath, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)
	return rr
}

func (s *streamHarness) seedAsset(t *testing.T, folder, filename, body string) string {
	t.Helper()
	urlPath := "/internal/conversations/" + s.convID.String() + "/assets/"
	if folder != "" {
		urlPath += folder + "/"
	}
	urlPath += filename
	rr := s.put(t, urlPath, bytes.NewReader([]byte(body)), "text/plain", s.runtimeSecret)
	if rr.Code != http.StatusCreated {
		t.Fatalf("seed asset: got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		PublicURL string `json:"asset_url"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	return resp.PublicURL
}

func TestDeleteAsset_HappyPath(t *testing.T) {
	h := newStreamHarness(t)
	publicURL := h.seedAsset(t, "tmp", "scratch.txt", "delete me")

	urlPath := fmt.Sprintf("/internal/conversations/%s/assets/tmp/scratch.txt", h.convID)
	rr := h.delete(t, urlPath, h.runtimeSecret)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Row gone.
	var count int64
	h.db.Model(&model.ConversationAsset{}).Where("key = ?", fmt.Sprintf("pub/c/%s/tmp/scratch.txt", h.convID)).Count(&count)
	if count != 0 {
		t.Fatalf("row still present after delete (count=%d)", count)
	}

	// S3 object gone.
	getReq, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, publicURL, nil)
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("public GET: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusNotFound && getResp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(getResp.Body)
		t.Fatalf("expected 404/403 after delete, got %d: %s", getResp.StatusCode, body)
	}
}

func TestDeleteAsset_NotFound(t *testing.T) {
	h := newStreamHarness(t)
	rr := h.delete(t,
		fmt.Sprintf("/internal/conversations/%s/assets/nope/missing.txt", h.convID),
		h.runtimeSecret,
	)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestDeleteAsset_BadBearer(t *testing.T) {
	h := newStreamHarness(t)
	h.seedAsset(t, "tmp", "x.txt", "hi")
	rr := h.delete(t,
		fmt.Sprintf("/internal/conversations/%s/assets/tmp/x.txt", h.convID),
		"wrong-key",
	)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMoveAsset_ByRelativePath(t *testing.T) {
	h := newStreamHarness(t)
	h.seedAsset(t, "videos", "demo.mp4", "fake mp4")

	body := `{"asset":"videos/demo.mp4","new_path":"archive/2026"}`
	rr := h.post(t,
		fmt.Sprintf("/internal/conversations/%s/assets/move", h.convID),
		body,
		h.runtimeSecret,
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Path     string `json:"path"`
		Key      string `json:"key"`
		Filename string `json:"filename"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Path != "archive/2026" {
		t.Fatalf("path: got %q want archive/2026", resp.Path)
	}
	// Key stays at the original location — only the DB path label moves.
	wantKey := fmt.Sprintf("pub/c/%s/videos/demo.mp4", h.convID)
	if resp.Key != wantKey {
		t.Fatalf("key changed: got %q want %q", resp.Key, wantKey)
	}

	var row model.ConversationAsset
	if err := h.db.Where("key = ?", wantKey).First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if row.Path != "archive/2026" {
		t.Fatalf("row.Path = %q", row.Path)
	}
}

func TestMoveAsset_ByPublicURL(t *testing.T) {
	h := newStreamHarness(t)
	publicURL := h.seedAsset(t, "tmp", "doc.txt", "hi")

	body := fmt.Sprintf(`{"asset":%q,"new_path":""}`, publicURL)
	rr := h.post(t,
		fmt.Sprintf("/internal/conversations/%s/assets/move", h.convID),
		body,
		h.runtimeSecret,
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Path != "" {
		t.Fatalf("path: got %q want empty (root)", resp.Path)
	}
}

func TestMoveAsset_AssetNotFound(t *testing.T) {
	h := newStreamHarness(t)
	body := `{"asset":"ghosts/none.txt","new_path":"archive"}`
	rr := h.post(t,
		fmt.Sprintf("/internal/conversations/%s/assets/move", h.convID),
		body,
		h.runtimeSecret,
	)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestMoveAsset_RejectsForeignURL(t *testing.T) {
	h := newStreamHarness(t)
	body := fmt.Sprintf(`{"asset":"https://example.com/pub/c/%s/foo.txt","new_path":"archive"}`, uuid.New())
	rr := h.post(t,
		fmt.Sprintf("/internal/conversations/%s/assets/move", h.convID),
		body,
		h.runtimeSecret,
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestMoveAsset_RejectsTraversalNewPath(t *testing.T) {
	h := newStreamHarness(t)
	h.seedAsset(t, "tmp", "x.txt", "hi")
	body := `{"asset":"tmp/x.txt","new_path":"../escape"}`
	rr := h.post(t,
		fmt.Sprintf("/internal/conversations/%s/assets/move", h.convID),
		body,
		h.runtimeSecret,
	)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestMoveAsset_BadBearer(t *testing.T) {
	h := newStreamHarness(t)
	body := `{"asset":"tmp/x.txt","new_path":"archive"}`
	rr := h.post(t,
		fmt.Sprintf("/internal/conversations/%s/assets/move", h.convID),
		body,
		"not-the-key",
	)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
