package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/enqueue"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/sandbox"
	"github.com/ziraloop/ziraloop/internal/tasks"
)

func commandsToString(cmds []string) string {
	return strings.Join(cmds, "\n")
}

func commandsToArray(cmdStr string) []string {
	if cmdStr == "" {
		return []string{}
	}
	return strings.Split(cmdStr, "\n")
}

// TemplateBuildable is the interface for building sandbox templates.
type TemplateBuildable interface {
	BuildTemplate(ctx context.Context, tmpl *model.SandboxTemplate)
	DeleteTemplate(ctx context.Context, externalID string) error
}

// Ensure sandbox.Orchestrator satisfies TemplateBuildable.
var _ TemplateBuildable = (*sandbox.Orchestrator)(nil)

type SandboxTemplateHandler struct {
	db       *gorm.DB
	builder  TemplateBuildable // nil if sandbox orchestrator not configured
	enqueuer enqueue.TaskEnqueuer
}

func NewSandboxTemplateHandler(db *gorm.DB, builder TemplateBuildable, enqueuer enqueue.TaskEnqueuer) *SandboxTemplateHandler {
	return &SandboxTemplateHandler{db: db, builder: builder, enqueuer: enqueuer}
}

type createSandboxTemplateRequest struct {
	Name           string     `json:"name"`
	BuildCommands  []string   `json:"build_commands"`
	Config         model.JSON `json:"config,omitempty"`
	BaseTemplateID *string    `json:"base_template_id,omitempty"` // UUID of a public template to use as base
}

type updateSandboxTemplateRequest struct {
	Name          *string    `json:"name,omitempty"`
	BuildCommands []string   `json:"build_commands,omitempty"`
	Config        model.JSON `json:"config,omitempty"`
}

type retryBuildRequest struct {
	BuildCommands []string `json:"build_commands,omitempty"`
}

type sandboxTemplateResponse struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Tags           model.JSON `json:"tags"`
	Size           string     `json:"size"`
	IsPublic       bool       `json:"is_public"`
	BaseTemplateID *string    `json:"base_template_id,omitempty"`
	BuildCommands  []string   `json:"build_commands"`
	ExternalID     *string    `json:"external_id,omitempty"`
	BuildStatus    string     `json:"build_status"`
	BuildError     *string    `json:"build_error,omitempty"`
	BuildLogs      string     `json:"build_logs,omitempty"`
	Config         model.JSON `json:"config"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
}

func toSandboxTemplateResponse(t model.SandboxTemplate) sandboxTemplateResponse {
	cmds := []string{}
	if t.BuildCommands != "" {
		cmds = []string{t.BuildCommands}
	}
	resp := sandboxTemplateResponse{
		ID:            t.ID.String(),
		Name:          t.Name,
		Slug:          t.Slug,
		Tags:          t.Tags,
		Size:          t.Size,
		IsPublic:      t.OrgID == nil,
		BuildCommands: cmds,
		ExternalID:    t.ExternalID,
		BuildStatus:   t.BuildStatus,
		BuildError:    t.BuildError,
		BuildLogs:     t.BuildLogs,
		Config:        t.Config,
		CreatedAt:     t.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     t.UpdatedAt.Format(time.RFC3339),
	}
	if t.BaseTemplateID != nil {
		baseIDStr := t.BaseTemplateID.String()
		resp.BaseTemplateID = &baseIDStr
	}
	return resp
}

// Create handles POST /v1/sandbox-templates.
// @Summary Create a sandbox template
// @Description Creates a new sandbox template with build commands.
// @Tags sandbox-templates
// @Accept json
// @Produce json
// @Param body body createSandboxTemplateRequest true "Template details"
// @Success 201 {object} sandboxTemplateResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandbox-templates [post]
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
		OrgID:         &org.ID,
		Name:          req.Name,
		BuildCommands: commandsToString(req.BuildCommands),
		BuildStatus:   "pending",
		Config:        req.Config,
		Tags:          model.JSON{},
	}
	if tmpl.Config == nil {
		tmpl.Config = model.JSON{}
	}

	// If a base template is specified, validate it's a public ready template and inherit its size.
	if req.BaseTemplateID != nil && *req.BaseTemplateID != "" {
		baseID, err := uuid.Parse(*req.BaseTemplateID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid base_template_id"})
			return
		}
		var baseTmpl model.SandboxTemplate
		if err := h.db.Where("id = ? AND org_id IS NULL AND build_status = ?", baseID, "ready").First(&baseTmpl).Error; err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "base template not found or not ready"})
			return
		}
		tmpl.BaseTemplateID = &baseID
		tmpl.Size = baseTmpl.Size
	}

	// Auto-generate slug from ID after creation
	tmpl.Slug = fmt.Sprintf("zira-tmpl-%s", uuid.New().String()[:8])

	if err := h.db.Create(&tmpl).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create sandbox template"})
		return
	}

	writeJSON(w, http.StatusCreated, toSandboxTemplateResponse(tmpl))
}

// List handles GET /v1/sandbox-templates.
// @Summary List sandbox templates
// @Description Returns sandbox templates for the current organization.
// @Tags sandbox-templates
// @Produce json
// @Param limit query int false "Page size (default 50, max 100)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[sandboxTemplateResponse]
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandbox-templates [get]
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

	q := h.db.Where("org_id = ? OR org_id IS NULL", org.ID)
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
		// Reset build status when commands change
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

	// Rebuild if commands changed
	if commandsChanged && h.builder != nil && tmpl.BuildCommands != "" {
		go h.builder.BuildTemplate(context.Background(), &tmpl)
	}

	writeJSON(w, http.StatusOK, toSandboxTemplateResponse(tmpl))
}

// TriggerBuild handles POST /v1/sandbox-templates/{id}/build.
// @Summary Trigger a sandbox template build
// @Description Enqueues an async build job for the template. Poll GET endpoint for status and logs.
// @Tags sandbox-templates
// @Accept json
// @Produce json
// @Param id path string true "Template ID"
// @Success 202 {object} sandboxTemplateResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandbox-templates/{id}/build [post]
func (h *SandboxTemplateHandler) TriggerBuild(w http.ResponseWriter, r *http.Request) {
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

	if tmpl.BuildStatus == "building" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "build already in progress"})
		return
	}

	if h.enqueuer == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "build worker not configured"})
		return
	}

	task, err := tasks.NewSandboxTemplateBuildTask(tmpl.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue build task"})
		return
	}
	if _, err := h.enqueuer.Enqueue(task); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue build task"})
		return
	}

	// Update status to building
	h.db.Model(&tmpl).Update("build_status", "building")
	tmpl.BuildStatus = "building"

	writeJSON(w, http.StatusAccepted, toSandboxTemplateResponse(tmpl))
}

// RetryBuild handles POST /v1/sandbox-templates/{id}/retry.
// @Summary Retry a sandbox template build
// @Description Deletes the existing snapshot (if any) and starts a new build. Can optionally update build commands.
// @Tags sandbox-templates
// @Accept json
// @Produce json
// @Param id path string true "Template ID"
// @Param body body retryBuildRequest false "Optional build commands update"
// @Success 202 {object} sandboxTemplateResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandbox-templates/{id}/retry [post]
func (h *SandboxTemplateHandler) RetryBuild(w http.ResponseWriter, r *http.Request) {
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

	if tmpl.BuildStatus == "building" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "build already in progress"})
		return
	}

	if h.enqueuer == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "build worker not configured"})
		return
	}

	var req retryBuildRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
	}

	// Update status to building and reset synchronously to prevent race conditions
	h.db.Model(&tmpl).Updates(map[string]any{
		"build_status": "building",
		"build_error":  nil,
		"build_logs":   "",
	})
	tmpl.BuildStatus = "building"
	tmpl.BuildError = nil
	tmpl.BuildLogs = ""

	task, err := tasks.NewSandboxTemplateRetryBuildTask(tmpl.ID, req.BuildCommands)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue retry task"})
		return
	}
	if _, err := h.enqueuer.Enqueue(task); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue retry task"})
		return
	}

	writeJSON(w, http.StatusAccepted, toSandboxTemplateResponse(tmpl))
}

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

	// Check if any agents reference this template
	var agentCount int64
	h.db.Model(&model.Agent{}).Where("sandbox_template_id = ? AND org_id = ?", id, org.ID).Count(&agentCount)
	if agentCount > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "cannot delete template: agents still reference it"})
		return
	}

	// Delete the Daytona snapshot if it was built
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
