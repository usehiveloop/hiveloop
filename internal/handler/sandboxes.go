package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/sandbox"
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
	IdentityID   string  `json:"identity_id"`
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
		IdentityID:   s.IdentityID.String(),
		SandboxType:  s.SandboxType,
		Status:       s.Status,
		ExternalID:   s.ExternalID,
		ErrorMessage: s.ErrorMessage,
		CreatedAt:    s.CreatedAt.Format(time.RFC3339),
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
