package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	ragdb "github.com/usehiveloop/hiveloop/internal/rag/db"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

type updateRAGSourceRequest struct {
	Name                *string     `json:"name,omitempty"`
	Status              *string     `json:"status,omitempty"`
	Enabled             *bool       `json:"enabled,omitempty"`
	Config              *model.JSON `json:"config,omitempty"`
	IndexingStart       *time.Time  `json:"indexing_start,omitempty"`
	RefreshFreqSeconds  *int        `json:"refresh_freq_seconds,omitempty"`
	PruneFreqSeconds    *int        `json:"prune_freq_seconds,omitempty"`
	PermSyncFreqSeconds *int        `json:"perm_sync_freq_seconds,omitempty"`
}

// Port of cc_pair.py:259 (status flip), :343 (rename), :372 (freq) —
// collapsed into one PATCH because Hiveloop has no separate
// status/property endpoints.
func (h *RAGSourceHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	srcID, ok := parseSourceID(chi.URLParam(r, "id"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid source id"})
		return
	}

	var req updateRAGSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	src, err := ragdb.GetSourceForOrg(h.db, org.ID, srcID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "source not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load source"})
		return
	}

	updates, status, msg := buildSourceUpdates(src, req)
	if status != 0 {
		writeJSON(w, status, errorResponse{Error: msg})
		return
	}
	if len(updates) == 0 {
		writeJSON(w, http.StatusOK, toRAGSourceResponse(src))
		return
	}

	if err := h.db.Model(src).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update source"})
		return
	}
	if err := h.db.Where("id = ?", src.ID).First(src).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to reload source"})
		return
	}

	writeJSON(w, http.StatusOK, toRAGSourceResponse(src))
}

func buildSourceUpdates(src *ragmodel.RAGSource, req updateRAGSourceRequest) (map[string]any, int, string) {
	updates := map[string]any{}

	if req.Name != nil {
		if *req.Name == "" {
			return nil, http.StatusBadRequest, "name cannot be empty"
		}
		updates["name"] = *req.Name
	}

	if req.Status != nil {
		st := ragmodel.RAGSourceStatus(*req.Status)
		if !isClientSettableStatus(st) {
			return nil, http.StatusUnprocessableEntity, "status must be ACTIVE or PAUSED"
		}
		updates["status"] = st
	}

	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if req.Config != nil {
		updates["config"] = *req.Config
	}

	if req.IndexingStart != nil {
		updates["indexing_start"] = *req.IndexingStart
	}

	if req.RefreshFreqSeconds != nil {
		clone := *src
		clone.RefreshFreqSeconds = req.RefreshFreqSeconds
		if err := clone.ValidateRefreshFreq(); err != nil {
			return nil, http.StatusUnprocessableEntity, err.Error()
		}
		updates["refresh_freq_seconds"] = *req.RefreshFreqSeconds
	}

	if req.PruneFreqSeconds != nil {
		clone := *src
		clone.PruneFreqSeconds = req.PruneFreqSeconds
		if err := clone.ValidatePruneFreq(); err != nil {
			return nil, http.StatusUnprocessableEntity, err.Error()
		}
		updates["prune_freq_seconds"] = *req.PruneFreqSeconds
	}

	if req.PermSyncFreqSeconds != nil {
		updates["perm_sync_freq_seconds"] = *req.PermSyncFreqSeconds
	}

	return updates, 0, ""
}

func isClientSettableStatus(s ragmodel.RAGSourceStatus) bool {
	return s == ragmodel.RAGSourceStatusActive || s == ragmodel.RAGSourceStatusPaused
}
