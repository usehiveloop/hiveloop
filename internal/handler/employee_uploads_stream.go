package handler

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// authEmployee resolves the employee agent + its sandbox from the URL param
// and verifies the bearer matches the sandbox's bridge API key. On failure
// it writes the JSON error response and returns false — callers must return.
func (h *UploadsHandler) authEmployee(w http.ResponseWriter, r *http.Request) (*model.Agent, *model.Sandbox, bool) {
	if h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "asset endpoints not configured"})
		return nil, nil, false
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "employeeID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid employee_id"})
		return nil, nil, false
	}

	bearer := bearerFromHeader(r.Header.Get("Authorization"))
	if bearer == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing authorization"})
		return nil, nil, false
	}

	var agent model.Agent
	if err := h.db.Where("id = ? AND status <> ?", agentID, "archived").First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
			return nil, nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load employee"})
		return nil, nil, false
	}

	var sandbox model.Sandbox
	if err := h.db.
		Where("agent_id = ? AND status NOT IN (?, ?)", agentID, "archived", "error").
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
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "decrypt bridge api key", "agent_id", agentID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify credentials"})
		return nil, nil, false
	}
	if subtle.ConstantTimeCompare([]byte(bearer), []byte(wantKey)) != 1 && !h.bearerMatchesEmployeeSubagentSandbox(r, agent.ID, bearer) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid bridge api key"})
		return nil, nil, false
	}

	return &agent, &sandbox, true
}

func (h *UploadsHandler) bearerMatchesEmployeeSubagentSandbox(r *http.Request, employeeID uuid.UUID, bearer string) bool {
	if bearer == "" || h.encKey == nil {
		return false
	}
	var sandboxes []model.Sandbox
	if err := h.db.
		Joins("JOIN specialist_tasks ON specialist_tasks.sandbox_id = sandboxes.id").
		Where("specialist_tasks.employee_agent_id = ? AND sandboxes.status NOT IN (?, ?)", employeeID, "archived", "error").
		Find(&sandboxes).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load employee subagent sandboxes for asset auth", "employee_id", employeeID, "error", err)
		return false
	}
	for _, sandbox := range sandboxes {
		wantKey, err := h.encKey.DecryptString(sandbox.EncryptedBridgeAPIKey)
		if err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "decrypt subagent bridge api key", "employee_id", employeeID, "sandbox_id", sandbox.ID, "error", err)
			continue
		}
		if subtle.ConstantTimeCompare([]byte(bearer), []byte(wantKey)) == 1 {
			return true
		}
	}
	return false
}

func buildEmployeeAssetKey(agentID uuid.UUID, folder, filename string) string {
	if folder == "" {
		return fmt.Sprintf("pub/e/%s/%s", agentID, filename)
	}
	return fmt.Sprintf("pub/e/%s/%s/%s", agentID, folder, filename)
}

// StreamEmployeeAsset accepts a streamed PUT body and stores it under the
// employee's drive. Auth: bearer must equal the employee sandbox's bridge
// API key.
//
//	PUT /internal/employees/{employeeID}/assets/*
func (h *UploadsHandler) StreamEmployeeAsset(w http.ResponseWriter, r *http.Request) {
	if h.streamer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "streaming uploads not configured"})
		return
	}

	folder, filename, perr := splitAssetPath(chi.URLParam(r, "*"))
	if perr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": perr.Error()})
		return
	}

	agent, sandbox, ok := h.authEmployee(w, r)
	if !ok {
		return
	}

	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if contentType == "" || contentType == "application/octet-stream" {
		if guessed := mime.TypeByExtension(filepath.Ext(filename)); guessed != "" {
			contentType = guessed
		}
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	key := buildEmployeeAssetKey(agent.ID, folder, filename)

	stored, err := h.streamer.Stream(r.Context(), key, contentType, r.Body)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "employee asset stream failed", "agent_id", agent.ID, "key", key, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upload failed"})
		return
	}

	asset := model.EmployeeAsset{
		AgentID:     agent.ID,
		OrgID:       *agent.OrgID,
		SandboxID:   sandbox.ID,
		Path:        folder,
		Filename:    filename,
		Key:         stored.Key,
		PublicURL:   stored.PublicURL,
		ContentType: contentType,
		Bytes:       stored.Bytes,
	}
	if err := h.db.Where("key = ?", stored.Key).Assign(map[string]any{
		"agent_id":            agent.ID,
		"org_id":              *agent.OrgID,
		"sandbox_id":          sandbox.ID,
		"path":                folder,
		"filename":            filename,
		assetURLStorageColumn: stored.PublicURL,
		"content_type":        contentType,
		"bytes":               stored.Bytes,
		"updated_at":          time.Now(),
	}).FirstOrCreate(&asset).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "persist employee asset", "agent_id", agent.ID, "key", stored.Key, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save asset"})
		return
	}

	writeJSON(w, http.StatusCreated, streamAssetResponse{
		ID:          asset.ID,
		PublicURL:   stored.PublicURL,
		Key:         stored.Key,
		Path:        folder,
		Filename:    filename,
		ContentType: contentType,
		Bytes:       stored.Bytes,
	})
}

// DeleteEmployeeAsset removes both the S3 object and the DB row.
//
//	DELETE /internal/employees/{employeeID}/assets/*
func (h *UploadsHandler) DeleteEmployeeAsset(w http.ResponseWriter, r *http.Request) {
	if h.streamer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "asset endpoints not configured"})
		return
	}

	folder, filename, perr := splitAssetPath(chi.URLParam(r, "*"))
	if perr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": perr.Error()})
		return
	}

	agent, _, ok := h.authEmployee(w, r)
	if !ok {
		return
	}

	key := buildEmployeeAssetKey(agent.ID, folder, filename)

	var asset model.EmployeeAsset
	if err := h.db.Where("agent_id = ? AND key = ?", agent.ID, key).First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "asset not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load asset"})
		return
	}

	if err := h.streamer.Delete(r.Context(), asset.Key); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "delete s3 object", "agent_id", agent.ID, "key", asset.Key, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to delete object"})
		return
	}
	if err := h.db.Delete(&asset).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "delete asset row", "agent_id", agent.ID, "key", asset.Key, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete asset"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
