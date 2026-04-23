package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// Get handles GET /v1/sandbox-templates/{id}.
// @Summary Get a sandbox template
// @Description Returns a single sandbox template by ID.
// @Tags sandbox-templates
// @Produce json
// @Param id path string true "Template ID"
// @Success 200 {object} sandboxTemplateResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandbox-templates/{id} [get]
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
// @Summary Update a sandbox template
// @Description Updates a sandbox template. Resets build status if commands change.
// @Tags sandbox-templates
// @Accept json
// @Produce json
// @Param id path string true "Template ID"
// @Param body body updateSandboxTemplateRequest true "Fields to update"
// @Success 200 {object} sandboxTemplateResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandbox-templates/{id} [put]
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
		updates["build_commands"] = commandsToString(req.BuildCommands)
		updates["build_status"] = "pending"
		updates["external_id"] = nil
		updates["build_error"] = nil
	}
	if req.Config != nil {
		updates["config"] = req.Config
	}

	commandsChanged := req.BuildCommands != nil

	if len(updates) > 0 {
		if err := h.db.Model(&tmpl).Updates(updates).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update sandbox template"})
			return
		}
		h.db.Where("id = ?", tmpl.ID).First(&tmpl)
	}

	if commandsChanged && h.builder != nil && tmpl.BuildCommands != "" {
		go h.builder.BuildTemplate(r.Context(), &tmpl)
	}

	writeJSON(w, http.StatusOK, toSandboxTemplateResponse(tmpl))
}
