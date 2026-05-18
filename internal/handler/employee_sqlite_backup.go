package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const defaultEmployeeSQLiteBackupMaxBytes int64 = 5 * 1024 * 1024 * 1024
const employeeSQLiteBackupReadTimeout = 10 * time.Minute

type employeeSQLiteBackupStreamer interface {
	Stream(ctx context.Context, key string, body io.Reader, contentType string) error
}

type EmployeeSQLiteBackupHandler struct {
	db       *gorm.DB
	storage  employeeSQLiteBackupStreamer
	encKey   *crypto.SymmetricKey
	maxBytes int64
}

func NewEmployeeSQLiteBackupHandler(db *gorm.DB, s3 employeeSQLiteBackupStreamer, encKey *crypto.SymmetricKey, maxBytes int64) *EmployeeSQLiteBackupHandler {
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
	if setEmployeeSQLiteBackupReadDeadline(w, time.Now().Add(employeeSQLiteBackupReadTimeout)) {
		defer setEmployeeSQLiteBackupReadDeadline(w, time.Time{})
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

	upgradeID, ok := h.parseAndVerifyUpgradeID(w, r, agent)
	if !ok {
		return
	}

	key := employeeSQLiteBackupKey(*agent.OrgID, agent.ID, upgradeID)
	body := http.MaxBytesReader(w, r.Body, h.maxBytes)
	if err := h.storage.Stream(r.Context(), key, body, "application/gzip"); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "backup exceeds maximum size"})
			return
		}
		attrs := []any{
			"employee_id", agent.ID,
			"sandbox_id", sandbox.ID,
			"key", key,
			"error", err,
		}
		if upgradeID != nil {
			attrs = append(attrs, "upgrade_id", *upgradeID)
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "employee sqlite backup upload failed", attrs...)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upload failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "key": key})
}

func setEmployeeSQLiteBackupReadDeadline(w http.ResponseWriter, deadline time.Time) bool {
	return http.NewResponseController(w).SetReadDeadline(deadline) == nil
}

func employeeSQLiteBackupKey(orgID, agentID uuid.UUID, upgradeID *uuid.UUID) string {
	if upgradeID != nil {
		return fmt.Sprintf("employee-sqlite-backups/%s/%s/upgrades/%s.db.gz", orgID, agentID, *upgradeID)
	}
	return fmt.Sprintf("employee-sqlite-backups/%s/%s/latest.db.gz", orgID, agentID)
}

func (h *EmployeeSQLiteBackupHandler) parseAndVerifyUpgradeID(w http.ResponseWriter, r *http.Request, agent *model.Agent) (*uuid.UUID, bool) {
	raw := r.URL.Query().Get("upgrade_id")
	if raw == "" {
		return nil, true
	}
	upgradeID, err := uuid.Parse(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid upgrade_id"})
		return nil, false
	}
	var count int64
	err = h.db.WithContext(r.Context()).Model(&model.EmployeeSandboxUpgrade{}).
		Where("id = ? AND org_id = ? AND agent_id = ?", upgradeID, *agent.OrgID, agent.ID).
		Count(&count).Error
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify upgrade"})
		return nil, false
	}
	if count == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "upgrade not found"})
		return nil, false
	}
	return &upgradeID, true
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
