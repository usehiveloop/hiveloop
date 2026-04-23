package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

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
			break
		}
		results = append(results, result)
	}

	h.db.Model(&sb).Update("last_active_at", time.Now())

	writeJSON(w, http.StatusOK, execResponse{
		Results: results,
		Success: allSuccess,
	})
}
