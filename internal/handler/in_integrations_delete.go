package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *InIntegrationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	var integ model.InIntegration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", integID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}

	nk := inNangoKey(integ.UniqueKey)
	slog.Info("deleting nango in-integration", "integration_id", integ.ID, "nango_key", nk)
	if err := h.nango.DeleteIntegration(r.Context(), nk); err != nil {
		slog.Error("nango in-integration deletion failed", "error", err, "integration_id", integ.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to delete integration from Nango: " + err.Error()})
		return
	}

	now := time.Now()
	if err := h.db.Model(&integ).Update("deleted_at", now).Error; err != nil {
		slog.Error("failed to soft-delete in-integration", "error", err, "integration_id", integ.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete integration"})
		return
	}

	slog.Info("in-integration deleted", "integration_id", integ.ID, "provider", integ.Provider)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
