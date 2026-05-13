package handler

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/storage"
)

const defaultEmployeeSQLiteBackupMaxBytes int64 = 512 * 1024 * 1024

type EmployeeSQLiteBackupHandler struct {
	db       *gorm.DB
	storage  *storage.S3Client
	encKey   *crypto.SymmetricKey
	maxBytes int64
}

func NewEmployeeSQLiteBackupHandler(db *gorm.DB, s3 *storage.S3Client, encKey *crypto.SymmetricKey, maxBytes int64) *EmployeeSQLiteBackupHandler {
	if maxBytes <= 0 {
		maxBytes = defaultEmployeeSQLiteBackupMaxBytes
	}
	return &EmployeeSQLiteBackupHandler{db: db, storage: s3, encKey: encKey, maxBytes: maxBytes}
}

func (h *EmployeeSQLiteBackupHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if h.storage == nil || h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sqlite backup endpoint not configured"})
		return
	}

	employeeID, err := uuid.Parse(chi.URLParam(r, "employeeID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee_id"})
		return
	}
	bearer := bearerFromHeader(r.Header.Get("Authorization"))
	if bearer == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/gzip" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content-type must be application/gzip"})
		return
	}
	if r.ContentLength > h.maxBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "backup exceeds maximum size"})
		return
	}

	agent, sandbox, ok := h.authenticateEmployeeBridge(w, r, employeeID, bearer)
	if !ok {
		return
	}

	key := fmt.Sprintf("employee-sqlite-backups/%s/%s/latest.db.gz", *agent.OrgID, agent.ID)
	body := http.MaxBytesReader(w, r.Body, h.maxBytes)
	if err := h.storage.Stream(r.Context(), key, body, "application/gzip"); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "backup exceeds maximum size"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "employee sqlite backup upload failed",
			"employee_id", agent.ID,
			"sandbox_id", sandbox.ID,
			"key", key,
			"error", err,
		)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upload failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "key": key})
}

func (h *EmployeeSQLiteBackupHandler) authenticateEmployeeBridge(w http.ResponseWriter, r *http.Request, employeeID uuid.UUID, bearer string) (*model.Agent, *model.Sandbox, bool) {
	var agent model.Agent
	if err := h.db.Where("id = ? AND is_employee = true", employeeID).First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return nil, nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return nil, nil, false
	}
	if agent.OrgID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "employee has no org"})
		return nil, nil, false
	}

	var sandbox model.Sandbox
	if err := h.db.
		Where("agent_id = ? AND status NOT IN (?, ?)", employeeID, "archived", "error").
		Order("created_at DESC").
		First(&sandbox).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found for employee"})
			return nil, nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load sandbox"})
		return nil, nil, false
	}

	wantKey, err := h.encKey.DecryptString(sandbox.EncryptedBridgeAPIKey)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "decrypt bridge api key", "agent_id", employeeID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify credentials"})
		return nil, nil, false
	}
	if subtle.ConstantTimeCompare([]byte(bearer), []byte(wantKey)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid bridge api key"})
		return nil, nil, false
	}
	return &agent, &sandbox, true
}
