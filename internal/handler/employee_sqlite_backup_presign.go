package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/storage"
)

const employeeSQLiteBackupPresignTTL = 15 * time.Minute

type employeeSQLiteBackupPresigner interface {
	PresignedPutURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	Head(ctx context.Context, key string) (*storage.S3ObjectInfo, error)
}

type employeeSQLiteBackupPresignRequest struct {
	Reason              string `json:"reason"`
	ContentType         string `json:"content_type"`
	CompressedBytesHint int64  `json:"compressed_bytes_hint"`
}

type employeeSQLiteBackupPresignResponse struct {
	Status    string    `json:"status"`
	Method    string    `json:"method"`
	Key       string    `json:"key"`
	UploadURL string    `json:"upload_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (h *EmployeeSQLiteBackupHandler) Presign(w http.ResponseWriter, r *http.Request) {
	presigner, ok := h.storage.(employeeSQLiteBackupPresigner)
	if h.storage == nil || h.encKey == nil || !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sqlite backup presign endpoint not configured"})
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
	var req employeeSQLiteBackupPresignRequest
	if r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid presign request"})
			return
		}
	}
	if req.ContentType != "" && req.ContentType != "application/gzip" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content_type must be application/gzip"})
		return
	}
	if req.CompressedBytesHint > h.maxBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "backup exceeds maximum size"})
		return
	}
	agent, _, ok := h.authenticateEmployeeBridge(w, r, employeeID, bearer)
	if !ok {
		return
	}
	key := employeeSQLiteBackupKey(*agent.OrgID, agent.ID, nil)
	uploadURL, err := presigner.PresignedPutURL(r.Context(), key, employeeSQLiteBackupPresignTTL)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "employee sqlite backup presign failed", "employee_id", agent.ID, "key", key, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "presign failed"})
		return
	}
	writeJSON(w, http.StatusOK, employeeSQLiteBackupPresignResponse{
		Status:    "ok",
		Method:    http.MethodPut,
		Key:       key,
		UploadURL: uploadURL,
		ExpiresAt: time.Now().UTC().Add(employeeSQLiteBackupPresignTTL),
	})
}

type employeeSQLiteBackupConfirmRequest struct {
	Key    string `json:"key"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	Reason string `json:"reason"`
	Writes uint64 `json:"writes"`
}

func (h *EmployeeSQLiteBackupHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	presigner, ok := h.storage.(employeeSQLiteBackupPresigner)
	if h.storage == nil || h.encKey == nil || !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sqlite backup confirm endpoint not configured"})
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
	var req employeeSQLiteBackupConfirmRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid confirm request"})
		return
	}
	agent, sandbox, ok := h.authenticateEmployeeBridge(w, r, employeeID, bearer)
	if !ok {
		return
	}
	expectedKey := employeeSQLiteBackupKey(*agent.OrgID, agent.ID, nil)
	if req.Key != expectedKey {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "backup key does not match employee"})
		return
	}
	info, err := presigner.Head(r.Context(), req.Key)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "employee sqlite backup confirm head failed", "employee_id", agent.ID, "sandbox_id", sandbox.ID, "key", req.Key, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "backup object not found"})
		return
	}
	if req.Bytes > 0 && info.Size != req.Bytes {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "backup object size mismatch"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "key": req.Key, "bytes": info.Size})
}
