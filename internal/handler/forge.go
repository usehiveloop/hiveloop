package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/enqueue"
	"github.com/ziraloop/ziraloop/internal/forge"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/streaming"
	"github.com/ziraloop/ziraloop/internal/tasks"
)

// ForgeHandler handles forge-related HTTP endpoints.
type ForgeHandler struct {
	db         *gorm.DB
	controller *forge.ForgeController
	eventBus   *streaming.EventBus
	enqueuer   enqueue.TaskEnqueuer
}

// NewForgeHandler creates a forge handler.
func NewForgeHandler(db *gorm.DB, controller *forge.ForgeController, eventBus *streaming.EventBus, enqueuer ...enqueue.TaskEnqueuer) *ForgeHandler {
	h := &ForgeHandler{db: db, controller: controller, eventBus: eventBus}
	if len(enqueuer) > 0 {
		h.enqueuer = enqueuer[0]
	}
	return h
}

type startForgeRequest struct {
	ArchitectCredentialID    string   `json:"architect_credential_id"`
	ArchitectModel           string   `json:"architect_model"`
	EvalDesignerCredentialID string   `json:"eval_designer_credential_id"`
	EvalDesignerModel        string   `json:"eval_designer_model"`
	JudgeCredentialID        string   `json:"judge_credential_id"`
	JudgeModel               string   `json:"judge_model"`
	MaxIterations            *int     `json:"max_iterations,omitempty"`
	PassThreshold            *float64 `json:"pass_threshold,omitempty"`
	ConvergenceLimit         *int     `json:"convergence_limit,omitempty"` // default 3
}

type forgeRunResponse struct {
	ID                string   `json:"id"`
	AgentID           string   `json:"agent_id"`
	Status            string   `json:"status"`
	CurrentIteration  int      `json:"current_iteration"`
	MaxIterations     int      `json:"max_iterations"`
	PassThreshold     float64  `json:"pass_threshold"`
	ConvergenceLimit  int      `json:"convergence_limit"`
	FinalScore        *float64 `json:"final_score,omitempty"`
	StopReason        string   `json:"stop_reason,omitempty"`
	TotalInputTokens  int      `json:"total_input_tokens"`
	TotalOutputTokens int      `json:"total_output_tokens"`
	TotalCost         float64  `json:"total_cost"`
	ErrorMessage      *string  `json:"error_message,omitempty"`
	StreamURL         string   `json:"stream_url"`
	StartedAt         *string  `json:"started_at,omitempty"`
	CompletedAt       *string  `json:"completed_at,omitempty"`
	CreatedAt         string   `json:"created_at"`
}

type forgeGetRunResponse struct {
	Run        forgeRunResponse         `json:"run"`
	Iterations []model.ForgeIteration   `json:"iterations"`
}

func toForgeRunResponse(run model.ForgeRun) forgeRunResponse {
	resp := forgeRunResponse{
		ID:                run.ID.String(),
		AgentID:           run.AgentID.String(),
		Status:            run.Status,
		CurrentIteration:  run.CurrentIteration,
		MaxIterations:     run.MaxIterations,
		PassThreshold:     run.PassThreshold,
		ConvergenceLimit:  run.ConvergenceLimit,
		FinalScore:        run.FinalScore,
		StopReason:        run.StopReason,
		TotalInputTokens:  run.TotalInputTokens,
		TotalOutputTokens: run.TotalOutputTokens,
		TotalCost:         run.TotalCost,
		ErrorMessage:      run.ErrorMessage,
		StreamURL:         fmt.Sprintf("/v1/forge-runs/%s/stream", run.ID),
		CreatedAt:         run.CreatedAt.Format(time.RFC3339),
	}
	if run.StartedAt != nil {
		s := run.StartedAt.Format(time.RFC3339)
		resp.StartedAt = &s
	}
	if run.CompletedAt != nil {
		s := run.CompletedAt.Format(time.RFC3339)
		resp.CompletedAt = &s
	}
	return resp
}

// Start creates and starts a new forge run.
// @Summary Start a forge run
// @Description Creates and starts a new forge run for the specified agent using the provided models, credentials, and configuration.
// @Tags forge
// @Accept json
// @Produce json
// @Param agentID path string true "Agent ID"
// @Param body body startForgeRequest true "Forge configuration"
// @Success 201 {object} forgeRunResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/forge [post]
func (h *ForgeHandler) Start(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
		return
	}

	var req startForgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Validate required fields.
	if req.ArchitectCredentialID == "" || req.ArchitectModel == "" ||
		req.EvalDesignerCredentialID == "" || req.EvalDesignerModel == "" ||
		req.JudgeCredentialID == "" || req.JudgeModel == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "architect_credential_id, architect_model, eval_designer_credential_id, eval_designer_model, judge_credential_id, and judge_model are required",
		})
		return
	}

	// Validate agent exists and belongs to org.
	var agent model.Agent
	if err := h.db.Where("id = ? AND org_id = ?", agentID, org.ID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load agent"})
		return
	}

	// Parse and validate credential UUIDs.
	archCredID, err := uuid.Parse(req.ArchitectCredentialID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid architect_credential_id"})
		return
	}
	evalCredID, err := uuid.Parse(req.EvalDesignerCredentialID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid eval_designer_credential_id"})
		return
	}
	judgeCredID, err := uuid.Parse(req.JudgeCredentialID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid judge_credential_id"})
		return
	}

	// Validate all 3 credentials belong to org and are not revoked.
	for _, credID := range []uuid.UUID{archCredID, evalCredID, judgeCredID} {
		var cred model.Credential
		if err := h.db.Where("id = ? AND org_id = ? AND revoked_at IS NULL", credID, org.ID).First(&cred).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": fmt.Sprintf("credential %s not found or revoked", credID),
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to validate credentials"})
			return
		}
	}

	// Set defaults.
	maxIter := 5
	if req.MaxIterations != nil && *req.MaxIterations > 0 {
		maxIter = *req.MaxIterations
	}
	threshold := 0.80
	if req.PassThreshold != nil && *req.PassThreshold > 0 && *req.PassThreshold <= 1.0 {
		threshold = *req.PassThreshold
	}
	convergenceLimit := 3
	if req.ConvergenceLimit != nil && *req.ConvergenceLimit > 0 {
		convergenceLimit = *req.ConvergenceLimit
	}

	slog.Info("forge: creating run",
		"forge_run_org_id", org.ID,
		"forge_run_agent_id", agentID,
		"forge_run_architect_model", req.ArchitectModel,
		"forge_run_eval_model", req.EvalDesignerModel,
		"forge_run_judge_model", req.JudgeModel,
		"forge_run_max_iterations", maxIter,
		"forge_run_pass_threshold", threshold,
	)

	// Create the forge run record.
	run := model.ForgeRun{
		OrgID:                    org.ID,
		AgentID:                  agentID,
		ArchitectCredentialID:    archCredID,
		ArchitectModel:           req.ArchitectModel,
		EvalDesignerCredentialID: evalCredID,
		EvalDesignerModel:        req.EvalDesignerModel,
		JudgeCredentialID:        judgeCredID,
		JudgeModel:               req.JudgeModel,
		MaxIterations:            maxIter,
		PassThreshold:            threshold,
		ConvergenceLimit:         convergenceLimit,
		Status:                   model.ForgeStatusQueued,
	}
	if err := h.db.Create(&run).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create forge run"})
		return
	}

	slog.Info("forge: run created, enqueueing",
		"forge_run_id", run.ID,
		"forge_run_agent_id", agentID,
		"forge_run_status", run.Status,
	)

	// Enqueue the forge run as an Asynq task.
	if h.enqueuer != nil {
		task, err := tasks.NewForgeRunTask(run.ID)
		if err == nil {
			info, err := h.enqueuer.Enqueue(task)
			if err != nil {
				slog.Error("forge: failed to enqueue run", "forge_run_id", run.ID, "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue forge run"})
				return
			}
			h.db.Model(&run).Update("asynq_task_id", info.ID)
			slog.Info("forge: run enqueued",
				"forge_run_id", run.ID,
				"asynq_task_id", info.ID,
				"asynq_queue", info.Queue,
			)
		}
	} else {
		slog.Info("forge: no enqueuer, running in goroutine", "forge_run_id", run.ID)
		go h.controller.Execute(context.Background(), run.ID)
	}

	writeJSON(w, http.StatusCreated, toForgeRunResponse(run))
}

// ListRuns lists forge runs for an agent.
// @Summary List forge runs
// @Description Returns all forge runs for the specified agent, ordered by creation date.
// @Tags forge
// @Produce json
// @Param agentID path string true "Agent ID"
// @Success 200 {array} forgeRunResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/forge [get]
func (h *ForgeHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	agentID := chi.URLParam(r, "agentID")

	var runs []model.ForgeRun
	h.db.Where("agent_id = ? AND org_id = ?", agentID, org.ID).
		Order("created_at DESC").
		Find(&runs)

	resp := make([]forgeRunResponse, len(runs))
	for i, run := range runs {
		resp[i] = toForgeRunResponse(run)
	}
	writeJSON(w, http.StatusOK, resp)
}

// GetRun returns a forge run with its iterations.
// @Summary Get forge run
// @Description Returns a forge run with all iterations and their details.
// @Tags forge
// @Produce json
// @Param runID path string true "Forge Run ID"
// @Success 200 {object} forgeGetRunResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/forge-runs/{runID} [get]
func (h *ForgeHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	runID := chi.URLParam(r, "runID")
	var run model.ForgeRun
	if err := h.db.Where("id = ? AND org_id = ?", runID, org.ID).First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "forge run not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load forge run"})
		return
	}

	// Load iterations.
	var iterations []model.ForgeIteration
	h.db.Where("forge_run_id = ?", run.ID).Order("iteration ASC").Find(&iterations)

	type iterResp struct {
		forgeRunResponse
		Iterations []model.ForgeIteration `json:"iterations"`
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"run":        toForgeRunResponse(run),
		"iterations": iterations,
	})
}

// Stream provides an SSE stream of forge events.
// @Summary Stream forge events
// @Description Real-time SSE stream of forge events for a forge run. Supports resume via Last-Event-ID.
// @Tags forge
// @Produce text/event-stream
// @Param runID path string true "Forge Run ID"
// @Header 200 {string} Last-Event-ID "ID of the last received event"
// @Success 200 {string} string "SSE stream"
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/forge-runs/{runID}/stream [get]
func (h *ForgeHandler) Stream(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	runID := chi.URLParam(r, "runID")
	var run model.ForgeRun
	if err := h.db.Where("id = ? AND org_id = ?", runID, org.ID).First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "forge run not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load forge run"})
		return
	}

	if h.eventBus == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "event streaming not available"})
		return
	}

	// Parse Last-Event-ID for resume.
	cursor := r.Header.Get("Last-Event-ID")
	if cursor == "" {
		cursor = "0"
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	streamKey := "forge:" + run.ID.String()
	events := h.eventBus.Subscribe(r.Context(), streamKey, cursor)

	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", event.ID, event.EventType, event.Data)
			flusher.Flush()
		case <-pingTicker.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// Cancel cancels a running forge.
// @Summary Cancel forge run
// @Description Cancels an active (queued or running) forge run.
// @Tags forge
// @Produce json
// @Param runID path string true "Forge Run ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/forge-runs/{runID}/cancel [post]
func (h *ForgeHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run_id"})
		return
	}

	var run model.ForgeRun
	if err := h.db.Where("id = ? AND org_id = ?", runID, org.ID).First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "forge run not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load forge run"})
		return
	}

	if run.Status != model.ForgeStatusRunning && run.Status != model.ForgeStatusQueued {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "forge run is not active"})
		return
	}

	h.controller.Cancel(runID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// Apply copies the forge result to the target agent.
// @Summary Apply forge result
// @Description Copies the best iteration's result to the target agent's configuration.
// @Tags forge
// @Produce json
// @Param runID path string true "Forge Run ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/forge-runs/{runID}/apply [post]
func (h *ForgeHandler) Apply(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	runID := chi.URLParam(r, "runID")
	var run model.ForgeRun
	if err := h.db.Where("id = ? AND org_id = ?", runID, org.ID).First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "forge run not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load forge run"})
		return
	}

	if run.Status != model.ForgeStatusCompleted {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "forge run is not completed"})
		return
	}
	if run.ResultSystemPrompt == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "forge run has no result"})
		return
	}

	// Apply results to the agent.
	updates := map[string]any{
		"system_prompt": *run.ResultSystemPrompt,
	}
	if len(run.ResultTools) > 0 {
		updates["tools"] = run.ResultTools
	}
	if len(run.ResultAgentConfig) > 0 {
		updates["agent_config"] = run.ResultAgentConfig
	}

	if err := h.db.Model(&model.Agent{}).Where("id = ?", run.AgentID).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update agent"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "applied"})
}

// ListEvents returns paginated forge events for a run.
// @Summary List forge events
// @Description Returns the audit trail of events for a forge run.
// @Tags forge
// @Produce json
// @Param runID path string true "Forge Run ID"
// @Success 200 {array} model.ForgeEvent
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/forge-runs/{runID}/events [get]
func (h *ForgeHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	runID := chi.URLParam(r, "runID")
	var run model.ForgeRun
	if err := h.db.Where("id = ? AND org_id = ?", runID, org.ID).First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "forge run not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load forge run"})
		return
	}

	var events []model.ForgeEvent
	h.db.Where("forge_run_id = ?", run.ID).Order("created_at ASC").Limit(500).Find(&events)
	writeJSON(w, http.StatusOK, events)
}

// ListEvals returns eval results for a specific iteration.
// @Summary List eval results
// @Description Returns eval results for all test cases in a specific forge iteration.
// @Tags forge
// @Produce json
// @Param runID path string true "Forge Run ID"
// @Param iterationID path string true "Iteration ID"
// @Success 200 {array} model.ForgeEvalResult
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/forge-runs/{runID}/iterations/{iterationID}/evals [get]
func (h *ForgeHandler) ListEvals(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	runID := chi.URLParam(r, "runID")
	iterID := chi.URLParam(r, "iterationID")

	// Verify the run belongs to the org.
	var run model.ForgeRun
	if err := h.db.Where("id = ? AND org_id = ?", runID, org.ID).First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "forge run not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load forge run"})
		return
	}

	var results []model.ForgeEvalResult
	h.db.Preload("ForgeEvalCase").Where("forge_iteration_id = ?", iterID).Order("created_at ASC").Find(&results)
	writeJSON(w, http.StatusOK, results)
}
