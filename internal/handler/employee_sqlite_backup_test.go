package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/storage"
)

type sqliteBackupHarness struct {
	db        *gorm.DB
	router    *chi.Mux
	s3        *storage.S3Client
	orgID     uuid.UUID
	agentID   uuid.UUID
	sandboxID uuid.UUID
	bridgeKey string
}

func newSQLiteBackupHarness(t *testing.T, maxBytes int64, isEmployee bool) *sqliteBackupHarness {
	t.Helper()
	s3 := newRealS3Client(t)
	return newSQLiteBackupHarnessWithStreamer(t, maxBytes, isEmployee, s3, s3)
}

type sqliteBackupStreamer interface {
	Stream(ctx context.Context, key string, body io.Reader, contentType string) error
}

func newSQLiteBackupHarnessWithStreamer(t *testing.T, maxBytes int64, isEmployee bool, streamer sqliteBackupStreamer, s3 *storage.S3Client) *sqliteBackupHarness {
	t.Helper()
	db := connectTestDB(t)
	encKey := testSymmetricKey(t)

	orgID := uuid.New()
	org := model.Org{
		ID:        orgID,
		Name:      fmt.Sprintf("sqlite-backup-%s", uuid.New().String()[:8]),
		RateLimit: 1000,
		Active:    true,
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", orgID).Delete(&model.Org{}) })

	agentID := uuid.New()
	agent := model.Agent{
		ID:           agentID,
		OrgID:        &orgID,
		Name:         fmt.Sprintf("employee-%s", uuid.New().String()[:8]),
		Status:       "active",
		Model:        "deepseek/deepseek-v4-flash",
		IsEmployee:   isEmployee,
		SystemPrompt: "",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	bridgeKey := "sqlite-backup-bridge-key-" + uuid.NewString()
	encryptedKey, err := encKey.EncryptString(bridgeKey)
	if err != nil {
		t.Fatalf("encrypt bridge key: %v", err)
	}
	sandboxID := uuid.New()
	sandbox := model.Sandbox{
		ID:                    sandboxID,
		OrgID:                 &orgID,
		AgentID:               &agentID,
		ExternalID:            "sqlite-backup-sandbox",
		BridgeURL:             "http://localhost:7080",
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "running",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	h := handler.NewEmployeeSQLiteBackupHandler(db, streamer, encKey, maxBytes)
	r := chi.NewRouter()
	r.Put("/internal/employees/{employeeID}/sqlite-backup", h.Upload)
	return &sqliteBackupHarness{
		db:        db,
		router:    r,
		s3:        s3,
		orgID:     orgID,
		agentID:   agentID,
		sandboxID: sandboxID,
		bridgeKey: bridgeKey,
	}
}

func newRealS3Client(t *testing.T) *storage.S3Client {
	t.Helper()
	endpoint := os.Getenv("PUBLIC_ASSETS_S3_ENDPOINT")
	if endpoint == "" {
		endpoint = testMinioEndpoint
	}
	hcReq, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, endpoint+"/minio/health/ready", nil)
	resp, err := http.DefaultClient.Do(hcReq)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Skipf("MinIO not reachable at %s: %v", endpoint, err)
	}
	_ = resp.Body.Close()

	s3, err := storage.NewS3Client(testMinioBucket, "auto", endpoint, testMinioAccess, testMinioSecret)
	if err != nil {
		t.Fatalf("create s3 client: %v", err)
	}
	return s3
}

func (h *sqliteBackupHarness) uploadBackup(t *testing.T, body []byte, bridgeKey string) *httptest.ResponseRecorder {
	t.Helper()
	return h.uploadBackupPath(t, "/internal/employees/"+h.agentID.String()+"/sqlite-backup", body, bridgeKey)
}

func (h *sqliteBackupHarness) uploadBackupPath(t *testing.T, path string, body []byte, bridgeKey string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/gzip")
	if bridgeKey != "" {
		req.Header.Set("Authorization", "Bearer "+bridgeKey)
	}
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *sqliteBackupHarness) backupKey() string {
	return fmt.Sprintf("employee-sqlite-backups/%s/%s/latest.db.gz", h.orgID, h.agentID)
}

func (h *sqliteBackupHarness) upgradeBackupKey(upgradeID uuid.UUID) string {
	return fmt.Sprintf("employee-sqlite-backups/%s/%s/upgrades/%s.db.gz", h.orgID, h.agentID, upgradeID)
}

func (h *sqliteBackupHarness) readBackupObject(t *testing.T) []byte {
	t.Helper()
	return h.readBackupKey(t, h.backupKey())
}

func (h *sqliteBackupHarness) readBackupKey(t *testing.T, key string) []byte {
	t.Helper()
	url, err := h.s3.PresignedURL(t.Context(), key, time.Minute)
	if err != nil {
		t.Fatalf("presign backup object: %v", err)
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get backup object: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get backup object status = %d", resp.StatusCode)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read backup object: %v", err)
	}
	return got
}

func TestEmployeeSQLiteBackup_UploadsFixedLatestObject(t *testing.T) {
	h := newSQLiteBackupHarness(t, 1024*1024, true)
	body := []byte("gzip-bytes-v1")

	rr := h.uploadBackup(t, body, h.bridgeKey)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["key"] != h.backupKey() {
		t.Fatalf("key = %q, want %q", resp["key"], h.backupKey())
	}
	if got := h.readBackupObject(t); !bytes.Equal(got, body) {
		t.Fatalf("uploaded object mismatch: %q", got)
	}
}

func TestEmployeeSQLiteBackup_SecondUploadOverwritesSameKey(t *testing.T) {
	h := newSQLiteBackupHarness(t, 1024*1024, true)
	first := []byte("first-backup")
	second := []byte("second-backup")

	if rr := h.uploadBackup(t, first, h.bridgeKey); rr.Code != http.StatusOK {
		t.Fatalf("first upload status = %d: %s", rr.Code, rr.Body.String())
	}
	if rr := h.uploadBackup(t, second, h.bridgeKey); rr.Code != http.StatusOK {
		t.Fatalf("second upload status = %d: %s", rr.Code, rr.Body.String())
	}
	if got := h.readBackupObject(t); !bytes.Equal(got, second) {
		t.Fatalf("backup was not overwritten: %q", got)
	}
}

func TestEmployeeSQLiteBackup_UpgradeIDWritesImmutableUpgradeKey(t *testing.T) {
	h := newSQLiteBackupHarness(t, 1024*1024, true)
	upgradeID := uuid.New()
	upgrade := model.EmployeeSandboxUpgrade{
		ID:           upgradeID,
		OrgID:        h.orgID,
		AgentID:      h.agentID,
		OldSandboxID: &h.sandboxID,
		Status:       model.EmployeeSandboxUpgradeStatusRunning,
		Phase:        model.EmployeeSandboxUpgradePhaseBackup,
	}
	if err := h.db.Create(&upgrade).Error; err != nil {
		t.Fatalf("create upgrade: %v", err)
	}

	body := []byte("upgrade-backup")
	path := "/internal/employees/" + h.agentID.String() + "/sqlite-backup?upgrade_id=" + upgradeID.String()
	rr := h.uploadBackupPath(t, path, body, h.bridgeKey)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	key := h.upgradeBackupKey(upgradeID)
	if resp["key"] != key {
		t.Fatalf("key = %q, want %q", resp["key"], key)
	}
	if got := h.readBackupKey(t, key); !bytes.Equal(got, body) {
		t.Fatalf("uploaded object mismatch: %q", got)
	}
}

func TestEmployeeSQLiteBackup_InvalidBearer(t *testing.T) {
	h := newSQLiteBackupHarness(t, 1024*1024, true)
	rr := h.uploadBackup(t, []byte("backup"), "wrong-key")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEmployeeSQLiteBackup_NonEmployeeRejected(t *testing.T) {
	h := newSQLiteBackupHarness(t, 1024*1024, false)
	rr := h.uploadBackup(t, []byte("backup"), h.bridgeKey)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestEmployeeSQLiteBackup_OversizedBodyRejected(t *testing.T) {
	h := newSQLiteBackupHarness(t, 4, true)
	rr := h.uploadBackup(t, []byte("too-large"), h.bridgeKey)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rr.Code, rr.Body.String())
	}
}
