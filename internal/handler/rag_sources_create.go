package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

const defaultRefreshFreqSeconds = 600

type createRAGSourceRequest struct {
	Kind                string     `json:"kind"`
	Name                string     `json:"name"`
	InConnectionID      *string    `json:"in_connection_id,omitempty"`
	AccessType          string     `json:"access_type"`
	Config              model.JSON `json:"config,omitempty"`
	RefreshFreqSeconds  *int       `json:"refresh_freq_seconds,omitempty"`
	PruneFreqSeconds    *int       `json:"prune_freq_seconds,omitempty"`
	PermSyncFreqSeconds *int       `json:"perm_sync_freq_seconds,omitempty"`
}

// @Summary Create a RAG source
// @Description Creates a new RAG source that the scheduler will pick up on the next tick. Kind=integration requires a valid in_connection_id pointing at an integration whose supports_rag_source flag is true. Refresh / prune / perm-sync frequencies are validated against per-org minimums.
// @Tags rag
// @Accept json
// @Produce json
// @Param body body createRAGSourceRequest true "Source definition"
// @Success 201 {object} ragSourceResponse
// @Security BearerAuth
// @Router /v1/rag/sources [post]
func (h *RAGSourceHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing user context"})
		return
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}

	var req createRAGSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	kind := ragmodel.RAGSourceKind(req.Kind)
	if !kind.IsValid() {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid kind"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "name is required"})
		return
	}

	access := ragmodel.AccessType(req.AccessType)
	switch access {
	case ragmodel.AccessTypePublic, ragmodel.AccessTypePrivate, ragmodel.AccessTypeSync:
	default:
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid access_type"})
		return
	}

	refresh := req.RefreshFreqSeconds
	if refresh == nil {
		// Default cadence keeps the source on the scheduler; the
		// selector skips sources with refresh_freq_seconds = NULL.
		d := defaultRefreshFreqSeconds
		refresh = &d
	}

	src := &ragmodel.RAGSource{
		ID:                  uuid.New(),
		OrgIDValue:          org.ID,
		KindValue:           kind,
		Name:                req.Name,
		Status:              ragmodel.RAGSourceStatusInitialIndexing,
		Enabled:             true,
		ConfigValue:         req.Config,
		AccessType:          access,
		RefreshFreqSeconds:  refresh,
		PruneFreqSeconds:    req.PruneFreqSeconds,
		PermSyncFreqSeconds: req.PermSyncFreqSeconds,
		CreatorID:           &user.ID,
	}

	if err := src.ValidateRefreshFreq(); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
		return
	}
	if err := src.ValidatePruneFreq(); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error()})
		return
	}

	if status, msg := h.attachInConnection(src, kind, req.InConnectionID, org.ID); status != 0 {
		writeJSON(w, status, errorResponse{Error: msg})
		return
	}

	if err := h.db.Create(src).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "a RAG source already exists for this connection"})
			return
		}
		slog.Error("failed to create rag source", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create source"})
		return
	}

	slog.Info("rag source created", "source_id", src.ID, "org_id", org.ID, "kind", src.KindValue)
	writeJSON(w, http.StatusCreated, toRAGSourceResponse(src))
}

func (h *RAGSourceHandler) attachInConnection(
	src *ragmodel.RAGSource,
	kind ragmodel.RAGSourceKind,
	rawID *string,
	orgID uuid.UUID,
) (int, string) {
	if kind != ragmodel.RAGSourceKindIntegration {
		if rawID != nil {
			return http.StatusBadRequest, "in_connection_id is only valid when kind=INTEGRATION"
		}
		return 0, ""
	}
	if rawID == nil || *rawID == "" {
		return http.StatusBadRequest, "in_connection_id is required when kind=INTEGRATION"
	}
	connID, err := uuid.Parse(*rawID)
	if err != nil {
		return http.StatusBadRequest, "invalid in_connection_id"
	}

	var conn model.InConnection
	if err := h.db.Preload("InIntegration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connID, orgID).
		First(&conn).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return http.StatusNotFound, "in_connection not found"
		}
		return http.StatusInternalServerError, "failed to load in_connection"
	}
	if conn.InIntegration.DeletedAt != nil {
		return http.StatusNotFound, "in_connection not found"
	}
	if !conn.InIntegration.SupportsRAGSource {
		return http.StatusUnprocessableEntity, "this integration does not support being used as a RAG source"
	}

	src.InConnectionID = &conn.ID
	return 0, ""
}
