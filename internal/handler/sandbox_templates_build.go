package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
)

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
