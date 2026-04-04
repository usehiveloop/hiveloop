package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/sandbox"
)

// SandboxHandler manages sandbox lifecycle via the API.
type SandboxHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
}

// NewSandboxHandler creates a sandbox handler.
func NewSandboxHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator) *SandboxHandler {
	return &SandboxHandler{db: db, orchestrator: orchestrator}
}

type sandboxResponse struct {
	ID           string  `json:"id"`
	IdentityID   *string `json:"identity_id,omitempty"`
	SandboxType  string  `json:"sandbox_type"`
	Status       string  `json:"status"`
	ExternalID   string  `json:"external_id"`
	AgentID      *string `json:"agent_id,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
	LastActiveAt *string `json:"last_active_at,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

func toSandboxResponse(s model.Sandbox) sandboxResponse {
	resp := sandboxResponse{
		ID:           s.ID.String(),
		SandboxType:  s.SandboxType,
		Status:       s.Status,
		ExternalID:   s.ExternalID,
		ErrorMessage: s.ErrorMessage,
		CreatedAt:    s.CreatedAt.Format(time.RFC3339),
	}
	if s.IdentityID != nil {
		id := s.IdentityID.String()
		resp.IdentityID = &id
	}
	if s.AgentID != nil {
		id := s.AgentID.String()
		resp.AgentID = &id
	}
	if s.LastActiveAt != nil {
		t := s.LastActiveAt.Format(time.RFC3339)
		resp.LastActiveAt = &t
	}
	return resp
}

// List handles GET /v1/sandboxes.
// @Summary List sandboxes
// @Description Returns sandboxes for the current organization.
// @Tags sandboxes
// @Produce json
// @Param status query string false "Filter by status (running, stopped, error)"
// @Param identity_id query string false "Filter by identity ID"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[sandboxResponse]
// @Security BearerAuth
// @Router /v1/sandboxes [get]
func (h *SandboxHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("org_id = ?", org.ID)
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	if identityID := r.URL.Query().Get("identity_id"); identityID != "" {
		q = q.Where("identity_id = ?", identityID)
	}
	q = applyPagination(q, cursor, limit)

	var sandboxes []model.Sandbox
	if err := q.Find(&sandboxes).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sandboxes"})
		return
	}

	hasMore := len(sandboxes) > limit
	if hasMore {
		sandboxes = sandboxes[:limit]
	}

	resp := make([]sandboxResponse, len(sandboxes))
	for i, s := range sandboxes {
		resp[i] = toSandboxResponse(s)
	}

	result := paginatedResponse[sandboxResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := sandboxes[len(sandboxes)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/sandboxes/{id}.
// @Summary Get a sandbox
// @Description Returns sandbox details by ID.
// @Tags sandboxes
// @Produce json
// @Param id path string true "Sandbox ID"
// @Success 200 {object} sandboxResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandboxes/{id} [get]
func (h *SandboxHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id := chi.URLParam(r, "id")
	var sb model.Sandbox
	if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox"})
		return
	}

	writeJSON(w, http.StatusOK, toSandboxResponse(sb))
}

// Stop handles POST /v1/sandboxes/{id}/stop.
// @Summary Stop a sandbox
// @Description Stops a running sandbox via the sandbox provider.
// @Tags sandboxes
// @Produce json
// @Param id path string true "Sandbox ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandboxes/{id}/stop [post]
func (h *SandboxHandler) Stop(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	if h.orchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	id := chi.URLParam(r, "id")
	var sb model.Sandbox
	if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox"})
		return
	}

	if err := h.orchestrator.StopSandbox(r.Context(), &sb); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to stop sandbox"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// Delete handles DELETE /v1/sandboxes/{id}.
// @Summary Delete a sandbox
// @Description Deletes a sandbox from the provider and removes the DB record.
// @Tags sandboxes
// @Produce json
// @Param id path string true "Sandbox ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandboxes/{id} [delete]
func (h *SandboxHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	if h.orchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	id := chi.URLParam(r, "id")
	var sb model.Sandbox
	if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox"})
		return
	}

	if err := h.orchestrator.DeleteSandbox(r.Context(), &sb); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete sandbox"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type execRequest struct {
	Commands []string `json:"commands"`
}

type commandResult struct {
	Command  string `json:"command"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

type execResponse struct {
	Results []commandResult `json:"results"`
	Success bool            `json:"success"`
}

// Exec handles POST /v1/sandboxes/{id}/exec.
// @Summary Execute commands in a sandbox
// @Description Runs an array of shell commands sequentially inside the sandbox. Stops on first failure.
// @Tags sandboxes
// @Accept json
// @Produce json
// @Param id path string true "Sandbox ID"
// @Param body body execRequest true "Commands to execute"
// @Success 200 {object} execResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandboxes/{id}/exec [post]
func (h *SandboxHandler) Exec(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id := chi.URLParam(r, "id")
	var sb model.Sandbox
	if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox"})
		return
	}

	if sb.Status != "running" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sandbox is not running"})
		return
	}

	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if len(req.Commands) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "commands array is required and must not be empty"})
		return
	}

	if h.orchestrator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox orchestrator not configured"})
		return
	}

	results := make([]commandResult, 0, len(req.Commands))
	allSuccess := true

	for _, cmd := range req.Commands {
		output, err := h.orchestrator.ExecuteCommand(r.Context(), &sb, cmd)
		result := commandResult{
			Command: cmd,
			Output:  output,
		}
		if err != nil {
			result.Error = err.Error()
			result.ExitCode = 1
			allSuccess = false
			slog.Debug("sandbox exec: command failed", "sandbox_id", sb.ID, "command", cmd, "error", err)
			results = append(results, result)
			break // stop on first failure
		}
		results = append(results, result)
	}

	h.db.Model(&sb).Update("last_active_at", time.Now())

	writeJSON(w, http.StatusOK, execResponse{
		Results: results,
		Success: allSuccess,
	})
}
