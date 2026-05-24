package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *IntegrationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	var integ model.Integration
	if err := h.db.Where("id = ? AND custom_app = false AND deleted_at IS NULL", integID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}
	if integ.ManagedBy != "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "managed integration is read-only"})
		return
	}

	nk := nangoProviderConfigKey(integ.UniqueKey)
	if err := h.nango.DeleteIntegration(r.Context(), nk); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "nango integration deletion failed", "error", err, "integration_id", integ.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to delete integration from Nango: " + err.Error()})
		return
	}

	now := time.Now()
	if err := h.db.Model(&integ).Update("deleted_at", now).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to soft-delete integration", "error", err, "integration_id", integ.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete integration"})
		return
	}

	logging.FromContext(r.Context()).InfoContext(r.Context(), "integration deleted", "integration_id", integ.ID, "provider", integ.Provider)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
