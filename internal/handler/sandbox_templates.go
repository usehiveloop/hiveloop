package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
)

type SandboxTemplateHandler struct {
	db *gorm.DB
}

func NewSandboxTemplateHandler(db *gorm.DB) *SandboxTemplateHandler {
	return &SandboxTemplateHandler{db: db}
}

type createSandboxTemplateRequest struct {
	Name          string     `json:"name"`
	BuildCommands string     `json:"build_commands"`
	Config        model.JSON `json:"config,omitempty"`
}

type updateSandboxTemplateRequest struct {
	Name          *string    `json:"name,omitempty"`
	BuildCommands *string    `json:"build_commands,omitempty"`
	Config        model.JSON `json:"config,omitempty"`
}

type sandboxTemplateResponse struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	BuildCommands string     `json:"build_commands"`
	ExternalID    *string    `json:"external_id,omitempty"`
	BuildStatus   string     `json:"build_status"`
	BuildError    *string    `json:"build_error,omitempty"`
	Config        model.JSON `json:"config"`
	CreatedAt     string     `json:"created_at"`
	UpdatedAt     string     `json:"updated_at"`
}

func toSandboxTemplateResponse(t model.SandboxTemplate) sandboxTemplateResponse {
	return sandboxTemplateResponse{
		ID:            t.ID.String(),
		Name:          t.Name,
		BuildCommands: t.BuildCommands,
		ExternalID:    t.ExternalID,
		BuildStatus:   t.BuildStatus,
		BuildError:    t.BuildError,
		Config:        t.Config,
		CreatedAt:     t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     t.UpdatedAt.Format(time.RFC3339),
	}
}

// Create handles POST /v1/sandbox-templates.
func (h *SandboxTemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createSandboxTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	tmpl := model.SandboxTemplate{
		OrgID:         org.ID,
		Name:          req.Name,
		BuildCommands: req.BuildCommands,
		BuildStatus:   "pending",
		Config:        req.Config,
	}
	if tmpl.Config == nil {
		tmpl.Config = model.JSON{}
	}

	if err := h.db.Create(&tmpl).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create sandbox template"})
		return
	}

	writeJSON(w, http.StatusCreated, toSandboxTemplateResponse(tmpl))
}

// List handles GET /v1/sandbox-templates.
func (h *SandboxTemplateHandler) List(w http.ResponseWriter, r *http.Request) {
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
	q = applyPagination(q, cursor, limit)

	var templates []model.SandboxTemplate
	if err := q.Find(&templates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sandbox templates"})
		return
	}

	hasMore := len(templates) > limit
	if hasMore {
		templates = templates[:limit]
	}

	resp := make([]sandboxTemplateResponse, len(templates))
	for i, t := range templates {
		resp[i] = toSandboxTemplateResponse(t)
	}

	result := paginatedResponse[sandboxTemplateResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := templates[len(templates)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/sandbox-templates/{id}.
func (h *SandboxTemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id := chi.URLParam(r, "id")
	var tmpl model.SandboxTemplate
	if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&tmpl).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox template not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox template"})
		return
	}

	writeJSON(w, http.StatusOK, toSandboxTemplateResponse(tmpl))
}

// Update handles PUT /v1/sandbox-templates/{id}.
func (h *SandboxTemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id := chi.URLParam(r, "id")
	var tmpl model.SandboxTemplate
	if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&tmpl).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox template not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox template"})
		return
	}

	var req updateSandboxTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.BuildCommands != nil {
		updates["build_commands"] = *req.BuildCommands
		// Reset build status when commands change
		updates["build_status"] = "pending"
		updates["external_id"] = nil
		updates["build_error"] = nil
	}
	if req.Config != nil {
		updates["config"] = req.Config
	}

	if len(updates) > 0 {
		if err := h.db.Model(&tmpl).Updates(updates).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update sandbox template"})
			return
		}
		h.db.Where("id = ?", tmpl.ID).First(&tmpl)
	}

	writeJSON(w, http.StatusOK, toSandboxTemplateResponse(tmpl))
}

// Delete handles DELETE /v1/sandbox-templates/{id}.
func (h *SandboxTemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id := chi.URLParam(r, "id")

	// Check if any agents reference this template
	var agentCount int64
	h.db.Model(&model.Agent{}).Where("sandbox_template_id = ? AND org_id = ?", id, org.ID).Count(&agentCount)
	if agentCount > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "cannot delete template: agents still reference it"})
		return
	}

	result := h.db.Where("id = ? AND org_id = ?", id, org.ID).Delete(&model.SandboxTemplate{})
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete sandbox template"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox template not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

