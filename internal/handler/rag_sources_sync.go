package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	ragdb "github.com/usehiveloop/hiveloop/internal/rag/db"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

// uniqueTriggerTTL bounds the "user clicks the button five times"
// dedup window for manual triggers. A user-initiated reindex that
// genuinely needs to fire twice will easily wait the minute.
const uniqueTriggerTTL = 60 * time.Second

type syncTriggerRequest struct {
	FromBeginning bool `json:"from_beginning,omitempty"`
}

type triggerResponse struct {
	TaskType     string `json:"task_type"`
	SourceID     string `json:"source_id"`
	Deduplicated bool   `json:"deduplicated"`
}

// @Summary Trigger an immediate ingest
// @Description Enqueues a one-off ingest job for the source. Set from_beginning=true to override the time-window floor and re-walk from the source's IndexingStart (or epoch).
// @Tags rag
// @Accept json
// @Produce json
// @Param id path string true "Source ID"
// @Param body body syncTriggerRequest false "Trigger options"
// @Success 202 {object} triggerResponse
// @Security BearerAuth
// @Router /v1/rag/sources/{id}/sync [post]
func (h *RAGSourceHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	src, status := h.loadSourceForTrigger(w, r)
	if status != 0 {
		return
	}

	var req syncTriggerRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	task, err := ragtasks.NewIngestTask(ragtasks.IngestPayload{
		RAGSourceID:   src.ID,
		FromBeginning: req.FromBeginning,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to build ingest task"})
		return
	}
	h.dispatchTrigger(w, src, task, ragtasks.TypeRagIngest)
}

// @Summary Trigger an immediate prune
// @Description Enqueues a one-off prune job: the connector enumerates upstream IDs and the worker deletes any documents we have that no longer exist upstream.
// @Tags rag
// @Produce json
// @Param id path string true "Source ID"
// @Success 202 {object} triggerResponse
// @Security BearerAuth
// @Router /v1/rag/sources/{id}/prune [post]
func (h *RAGSourceHandler) TriggerPrune(w http.ResponseWriter, r *http.Request) {
	src, status := h.loadSourceForTrigger(w, r)
	if status != 0 {
		return
	}

	task, err := ragtasks.NewPruneTask(ragtasks.PrunePayload{RAGSourceID: src.ID})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to build prune task"})
		return
	}
	h.dispatchTrigger(w, src, task, ragtasks.TypeRagPrune)
}

// @Summary Trigger an immediate permission sync
// @Description Enqueues a one-off permission-sync job. Returns 422 if the source's connector does not implement PermSyncConnector (i.e. has no external ACL model worth syncing).
// @Tags rag
// @Produce json
// @Param id path string true "Source ID"
// @Success 202 {object} triggerResponse
// @Security BearerAuth
// @Router /v1/rag/sources/{id}/perm-sync [post]
func (h *RAGSourceHandler) TriggerPermSync(w http.ResponseWriter, r *http.Request) {
	src, status := h.loadSourceForTrigger(w, r)
	if status != 0 {
		return
	}
	if h.caps == nil || !h.caps(string(src.KindValue)) {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{
			Error: "this connector does not support permission sync",
		})
		return
	}

	task, err := ragtasks.NewPermSyncTask(ragtasks.PermSyncPayload{RAGSourceID: src.ID})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to build perm_sync task"})
		return
	}
	h.dispatchTrigger(w, src, task, ragtasks.TypeRagPermSync)
}

func (h *RAGSourceHandler) loadSourceForTrigger(w http.ResponseWriter, r *http.Request) (*ragmodel.RAGSource, int) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, http.StatusUnauthorized
	}
	srcID, ok := parseSourceID(chi.URLParam(r, "id"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid source id"})
		return nil, http.StatusBadRequest
	}
	src, err := ragdb.GetSourceForOrg(h.db, org.ID, srcID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "source not found"})
			return nil, http.StatusNotFound
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load source"})
		return nil, http.StatusInternalServerError
	}
	if src.Status == ragmodel.RAGSourceStatusDeleting {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "source is being deleted"})
		return nil, http.StatusConflict
	}
	return src, 0
}

func (h *RAGSourceHandler) dispatchTrigger(
	w http.ResponseWriter,
	src *ragmodel.RAGSource,
	task *asynq.Task,
	taskType string,
) {
	_, err := h.enq.Enqueue(task, asynq.Unique(uniqueTriggerTTL))
	dedup := false
	if err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			dedup = true
		} else {
			slog.Error("rag trigger enqueue failed", "task_type", taskType, "source_id", src.ID, "err", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to enqueue task"})
			return
		}
	}

	writeJSON(w, http.StatusAccepted, triggerResponse{
		TaskType:     taskType,
		SourceID:     src.ID.String(),
		Deduplicated: dedup,
	})
}
