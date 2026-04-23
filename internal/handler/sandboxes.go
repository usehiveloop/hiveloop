package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

// SandboxHandler manages sandbox lifecycle via the API.
type SandboxHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
}

func NewSandboxHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator) *SandboxHandler {
	return &SandboxHandler{db: db, orchestrator: orchestrator}
}

type sandboxResponse struct {
	ID           string  `json:"id"`
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
