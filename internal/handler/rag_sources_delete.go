package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	ragdb "github.com/usehiveloop/hiveloop/internal/rag/db"
)

// @Summary Delete a RAG source
// @Description Hard-deletes the source row. Postgres-side rows (attempts, documents, sync state, ACLs) cascade. Vector store entries are reaped later by the prune loop.
// @Tags rag
// @Produce json
// @Param id path string true "Source ID"
// @Success 204
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

	if err := h.db.Delete(src).Error; err != nil {
		slog.Error("failed to delete rag source", "error", err, "source_id", src.ID, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to delete source"})
		return
	}

	slog.Info("rag source deleted", "source_id", src.ID, "org_id", org.ID)
	w.WriteHeader(http.StatusNoContent)
}
