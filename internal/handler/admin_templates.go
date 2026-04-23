package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// ListSandboxTemplates handles GET /admin/v1/sandbox-templates.
// @Summary List all sandbox templates
// @Description Returns sandbox templates across all organizations. Use scope=public to list only public templates.
// @Tags admin
// @Produce json
// @Param org_id query string false "Filter by org ID"
// @Param scope query string false "Filter by scope (public = org_id IS NULL)"
// @Param build_status query string false "Filter by build status (pending, building, ready, failed)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[adminSandboxTemplateResponse]
// @Security BearerAuth
// @Router /admin/v1/sandbox-templates [get]
func (h *AdminHandler) ListSandboxTemplates(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.SandboxTemplate{})
	if scope := r.URL.Query().Get("scope"); scope == "public" {
		q = q.Where("org_id IS NULL")
	} else if orgID := r.URL.Query().Get("org_id"); orgID != "" {
		q = q.Where("org_id = ?", orgID)
	}
	if status := r.URL.Query().Get("build_status"); status != "" {
		q = q.Where("build_status = ?", status)
	}

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

	resp := make([]adminSandboxTemplateResponse, len(templates))
	for i, t := range templates {
		resp[i] = toAdminSandboxTemplateResponse(t)
	}

	result := paginatedResponse[adminSandboxTemplateResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := templates[len(templates)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// DeleteSandboxTemplate handles DELETE /admin/v1/sandbox-templates/{id}.
// @Summary Delete a sandbox template
// @Description Permanently deletes a sandbox template.
// @Tags admin
// @Produce json
// @Param id path string true "Template ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandbox-templates/{id} [delete]
func (h *AdminHandler) DeleteSandboxTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var tmpl model.SandboxTemplate
	if err := h.db.Where("id = ?", id).First(&tmpl).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox template not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox template"})
		return
	}

	if err := h.db.Delete(&tmpl).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete sandbox template"})
		return
	}

	slog.Info("admin: sandbox template deleted", "template_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
// CreateSandboxTemplate handles POST /admin/v1/sandbox-templates.
// @Summary Register a public sandbox template
// @Description Registers a pre-built Daytona snapshot as a public (platform-wide) sandbox template.
// @Tags admin
// @Accept json
// @Produce json
// @Param body body adminCreateSandboxTemplateRequest true "Template details"
// @Success 201 {object} adminSandboxTemplateResponse
// @Failure 400 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandbox-templates [post]
func (h *AdminHandler) CreateSandboxTemplate(w http.ResponseWriter, r *http.Request) {
	var req adminCreateSandboxTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug is required (Daytona snapshot name)"})
		return
	}

	size := req.Size
	if size == "" {
		size = "medium"
	}
	if !model.ValidTemplateSize(size) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid size (valid: small, medium, large, xlarge)"})
		return
	}

	tags := model.JSON{}
	if len(req.Tags) > 0 {
		tagSlice := make([]any, len(req.Tags))
		for idx, tag := range req.Tags {
			tagSlice[idx] = tag
		}
		tagsJSON, _ := json.Marshal(tagSlice)
		_ = json.Unmarshal(tagsJSON, &tags)
	}

	tmpl := model.SandboxTemplate{
		OrgID:       nil, // public template
		Name:        name,
		Slug:        slug,
		Tags:        tags,
		Size:        size,
		ExternalID:  &slug, // slug IS the Daytona snapshot name
		BuildStatus: "ready",
		Config:      model.JSON{},
	}

	if err := h.db.Create(&tmpl).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create sandbox template"})
		return
	}

	slog.Info("admin: public sandbox template registered", "template_id", tmpl.ID, "name", name, "slug", slug, "size", size)
	writeJSON(w, http.StatusCreated, toAdminSandboxTemplateResponse(tmpl))
}

// GetSandboxTemplate handles GET /admin/v1/sandbox-templates/{id}.
// @Summary Get a sandbox template
// @Description Returns a single sandbox template by ID.
// @Tags admin
// @Produce json
// @Param id path string true "Template ID"
// @Success 200 {object} adminSandboxTemplateResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandbox-templates/{id} [get]
func (h *AdminHandler) GetSandboxTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var tmpl model.SandboxTemplate
	if err := h.db.Where("id = ?", id).First(&tmpl).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox template not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox template"})
		return
	}

	writeJSON(w, http.StatusOK, toAdminSandboxTemplateResponse(tmpl))
}
type adminUpdateSandboxTemplateRequest struct {
	Name *string  `json:"name,omitempty"`
	Slug *string  `json:"slug,omitempty"` // Daytona snapshot name
	Tags []string `json:"tags,omitempty"` // user-facing tags
	Size *string  `json:"size,omitempty"`
}

// UpdateSandboxTemplate handles PUT /admin/v1/sandbox-templates/{id}.
// @Summary Update a sandbox template
// @Description Updates sandbox template name, slug, tags, and size.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "Template ID"
// @Param body body adminUpdateSandboxTemplateRequest true "Fields to update"
// @Success 200 {object} adminSandboxTemplateResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/sandbox-templates/{id} [put]
func (h *AdminHandler) UpdateSandboxTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var tmpl model.SandboxTemplate
	if err := h.db.Where("id = ?", id).First(&tmpl).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox template not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox template"})
		return
	}

	var req adminUpdateSandboxTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]any{}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name cannot be empty"})
			return
		}
		updates["name"] = name
	}
	if req.Size != nil {
		if !model.ValidTemplateSize(*req.Size) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid size (valid: small, medium, large, xlarge)"})
			return
		}
		updates["size"] = *req.Size
	}
	if req.Slug != nil {
		slug := strings.TrimSpace(*req.Slug)
		if slug == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug cannot be empty"})
			return
		}
		updates["slug"] = slug
		updates["external_id"] = slug // slug IS the Daytona snapshot name
		updates["build_status"] = "ready"
	}
	if req.Tags != nil {
		tagsJSON, _ := json.Marshal(req.Tags)
		var tagsModel model.JSON
		_ = json.Unmarshal(tagsJSON, &tagsModel)
		updates["tags"] = tagsModel
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	old := map[string]any{"name": tmpl.Name}
	setAuditDiff(r, old, updates)

	if err := h.db.Model(&tmpl).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update sandbox template"})
		return
	}

	h.db.Where("id = ?", id).First(&tmpl)
	slog.Info("admin: sandbox template updated", "template_id", id)
	writeJSON(w, http.StatusOK, toAdminSandboxTemplateResponse(tmpl))
}