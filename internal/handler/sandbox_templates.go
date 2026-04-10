package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/enqueue"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/sandbox"
	"github.com/ziraloop/ziraloop/internal/streaming"
	"github.com/ziraloop/ziraloop/internal/tasks"
)

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
	eventBus *streaming.EventBus
}

func NewSandboxTemplateHandler(db *gorm.DB, builder TemplateBuildable, enqueuer enqueue.TaskEnqueuer, eventBus *streaming.EventBus) *SandboxTemplateHandler {
	return &SandboxTemplateHandler{db: db, builder: builder, enqueuer: enqueuer, eventBus: eventBus}
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

type buildTriggerResponse struct {
	StreamURL string `json:"stream_url"`
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

	// Trigger build in background if builder is configured and commands are provided
	if h.builder != nil && tmpl.BuildCommands != "" {
		go h.builder.BuildTemplate(context.Background(), &tmpl)
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
		updates["build_commands"] = *req.BuildCommands
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
// @Description Enqueues an async build job for the template and streams logs via SSE.
// @Tags sandbox-templates
// @Accept json
// @Produce json
// @Param id path string true "Template ID"
// @Success 202 {object} buildTriggerResponse
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

	writeJSON(w, http.StatusAccepted, buildTriggerResponse{
		StreamURL: fmt.Sprintf("/v1/sandbox-templates/%s/build-stream", tmpl.ID),
	})
}

// StreamBuildLogs handles GET /v1/sandbox-templates/{id}/build-stream.
// @Summary Stream template build logs
// @Description Real-time SSE stream of template build logs and status updates. Supports resume via Last-Event-ID.
// @Tags sandbox-templates
// @Produce text/event-stream
// @Param id path string true "Template ID"
// @Header 200 {string} Last-Event-ID "ID of the last received event"
// @Success 200 {string} string "SSE stream"
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandbox-templates/{id}/build-stream [get]
func (h *SandboxTemplateHandler) StreamBuildLogs(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		http.Error(w, "missing org context", http.StatusUnauthorized)
		return
	}

	id := chi.URLParam(r, "id")

	var tmpl model.SandboxTemplate
	if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&tmpl).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "sandbox template not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to get sandbox template", http.StatusInternalServerError)
		return
	}

	if h.eventBus == nil {
		http.Error(w, "streaming not configured", http.StatusInternalServerError)
		return
	}

	streamKey := fmt.Sprintf("template:%s", tmpl.ID.String())
	cursor := r.Header.Get("Last-Event-ID")
	if cursor == "" {
		cursor = "0"
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	events := h.eventBus.Subscribe(r.Context(), streamKey, cursor)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			eventID := event.ID
			_, _ = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", eventID, event.EventType, string(event.Data))
			flusher.Flush()
		case <-ticker.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
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
