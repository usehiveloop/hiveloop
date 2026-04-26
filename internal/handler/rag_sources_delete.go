package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	ragdb "github.com/usehiveloop/hiveloop/internal/rag/db"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

// @Summary Delete a RAG source
// @Description Soft-tombstones the source (Status → DELETING). The scheduler stops enqueuing work immediately; row + document teardown happens asynchronously via a cleanup loop.
// @Tags rag
// @Produce json
// @Param id path string true "Source ID"
// @Success 202 {object} triggerResponse
// @Security BearerAuth
// @Router /v1/rag/sources/{id} [delete]
func (h *RAGSourceHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	src, err := ragdb.GetSourceForOrg(h.db, org.ID, srcID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "source not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load source"})
		return
	}

	if src.Status == ragmodel.RAGSourceStatusDeleting {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status": "deleting",
			"note":   "documents will be removed asynchronously",
		})
		return
	}

	if err := h.db.Model(src).Update("status", ragmodel.RAGSourceStatusDeleting).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to mark source for deletion"})
		return
	}

	slog.Info("rag source marked for deletion", "source_id", src.ID, "org_id", org.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "deleting",
		"note":   "documents will be removed asynchronously",
	})
}
