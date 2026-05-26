package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func (h *sqliteBackupHarness) presignBackup(t *testing.T, body any, runtimeSecret string) *httptest.ResponseRecorder {
	t.Helper()
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal presign body: %v", err)
		}
		payload = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/employees/"+h.agentID.String()+"/sqlite-backup/presign", payload)
	if runtimeSecret != "" {
		req.Header.Set("Authorization", "Bearer "+runtimeSecret)
	}
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *sqliteBackupHarness) confirmBackup(t *testing.T, body any, runtimeSecret string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal confirm body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/employees/"+h.agentID.String()+"/sqlite-backup/confirm", bytes.NewReader(raw))
	if runtimeSecret != "" {
		req.Header.Set("Authorization", "Bearer "+runtimeSecret)
	}
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestEmployeeSQLiteBackup_PresignAllowsDirectStorageUploadAndConfirm(t *testing.T) {
	h := newSQLiteBackupHarness(t, 1024*1024, true)
	body := []byte("direct-to-storage-backup")

	rr := h.presignBackup(t, map[string]any{
		"reason":                "threshold",
		"content_type":          "application/gzip",
		"compressed_bytes_hint": len(body),
	}, h.runtimeSecret)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected presign 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var presign struct {
		Method    string `json:"method"`
		Key       string `json:"key"`
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &presign); err != nil {
		t.Fatalf("decode presign: %v", err)
	}
	if presign.Method != http.MethodPut || presign.Key != h.backupKey() || presign.UploadURL == "" {
		t.Fatalf("unexpected presign response: %#v", presign)
	}
	putReq, err := http.NewRequestWithContext(t.Context(), http.MethodPut, presign.UploadURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create direct put request: %v", err)
	}
	putReq.Header.Set("Content-Type", "application/gzip")
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatalf("direct put backup: %v", err)
	}
	_ = putResp.Body.Close()
	if putResp.StatusCode < 200 || putResp.StatusCode >= 300 {
		t.Fatalf("direct put status = %d", putResp.StatusCode)
	}

	confirm := h.confirmBackup(t, map[string]any{
		"key":    presign.Key,
		"bytes":  len(body),
		"sha256": "not-yet-validated-by-storage",
		"reason": "threshold",
		"writes": 1000,
	}, h.runtimeSecret)
	if confirm.Code != http.StatusOK {
		t.Fatalf("expected confirm 200, got %d: %s", confirm.Code, confirm.Body.String())
	}
	if got := h.readBackupObject(t); !bytes.Equal(got, body) {
		t.Fatalf("direct uploaded object mismatch: %q", got)
	}
}

func TestEmployeeSQLiteBackup_ConfirmRejectsCrossEmployeeKey(t *testing.T) {
	h := newSQLiteBackupHarness(t, 1024*1024, true)
	rr := h.confirmBackup(t, map[string]any{
		"key":   fmt.Sprintf("employee-sqlite-backups/%s/%s/latest.db.gz", uuid.New(), uuid.New()),
		"bytes": 10,
	}, h.runtimeSecret)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEmployeeSQLiteBackup_PresignRejectsOversizedBackupHint(t *testing.T) {
	h := newSQLiteBackupHarness(t, 4, true)
	rr := h.presignBackup(t, map[string]any{
		"content_type":          "application/gzip",
		"compressed_bytes_hint": 5,
	}, h.runtimeSecret)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rr.Code, rr.Body.String())
	}
}
