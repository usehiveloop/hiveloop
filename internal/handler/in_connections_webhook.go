package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// @Summary Mark webhook as configured
// @Description Sets the webhook_configured flag to true on a connection, indicating the user has manually configured the webhook URL in the provider's dashboard.
// @Tags in-connections
// @Produce json
// @Param id path string true "Connection ID"
// @Success 200 {object} inConnectionResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/in/connections/{id}/webhook-configured [patch]
func (h *InConnectionHandler) MarkWebhookConfigured(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	connID := chi.URLParam(r, "id")
	if connID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection id required"})
		return
	}

	result := h.db.Model(&model.InConnection{}).
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connID, org.ID).
		Update("webhook_configured", true)

	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update connection"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	var conn model.InConnection
	if err := h.db.Preload("InIntegration").Where("id = ?", connID).First(&conn).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reload connection"})
		return
	}

	writeJSON(w, http.StatusOK, h.toInConnectionResponse(conn))
}
