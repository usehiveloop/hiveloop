package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// Delete handles DELETE /v1/sandbox-templates/{id}.
// @Summary Delete a sandbox template
// @Description Deletes a template. Fails if agents still reference it.
// @Tags sandbox-templates
// @Produce json
// @Param id path string true "Template ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandbox-templates/{id} [delete]
func (h *SandboxTemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	id := chi.URLParam(r, "id")

	var agentCount int64
	h.db.Model(&model.Agent{}).Where("sandbox_template_id = ? AND org_id = ?", id, org.ID).Count(&agentCount)
	if agentCount > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "cannot delete template: agents still reference it"})
		return
	}

	var tmpl model.SandboxTemplate
	if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&tmpl).Error; err == nil {
		if tmpl.ExternalID != nil && h.builder != nil {
			_ = h.builder.DeleteTemplate(r.Context(), *tmpl.ExternalID)
		}
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

type publicTemplateResponse struct {
	ID   string     `json:"id"`
	Name string     `json:"name"`
	Slug string     `json:"slug"`
	Tags model.JSON `json:"tags"`
	Size string     `json:"size"`
}

// ListPublic handles GET /v1/sandbox-templates/public.
// @Summary List public sandbox templates
// @Description Returns all public (platform-wide) sandbox templates that are ready.
// @Tags sandbox-templates
// @Produce json
// @Success 200 {object} map[string][]publicTemplateResponse
// @Security BearerAuth
// @Router /v1/sandbox-templates/public [get]
func (h *SandboxTemplateHandler) ListPublic(w http.ResponseWriter, r *http.Request) {
	var templates []model.SandboxTemplate
	if err := h.db.Where("org_id IS NULL AND build_status = ?", "ready").
		Order("name ASC").Find(&templates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list public templates"})
		return
	}

	resp := make([]publicTemplateResponse, len(templates))
	for index, tmpl := range templates {
		resp[index] = publicTemplateResponse{
			ID:   tmpl.ID.String(),
			Name: tmpl.Name,
			Slug: tmpl.Slug,
			Tags: tmpl.Tags,
			Size: tmpl.Size,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}
