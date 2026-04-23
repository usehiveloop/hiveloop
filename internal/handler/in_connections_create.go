package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// @Summary Create an in-connection
// @Description Stores a connection after the OAuth flow completes via Nango.
// @Tags in-connections
// @Accept json
// @Produce json
// @Param id path string true "Integration ID"
// @Param body body createInConnectionRequest true "Connection details"
// @Success 201 {object} inConnectionResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/in/integrations/{id}/connections [post]
func (h *InConnectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	integUUID, err := uuid.Parse(integID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid integration id"})
		return
	}

	var integ model.InIntegration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", integUUID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}

	var req createInConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.NangoConnectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nango_connection_id is required"})
		return
	}

	conn := model.InConnection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		InIntegrationID:   integ.ID,
		NangoConnectionID: req.NangoConnectionID,
		Meta:              req.Meta,
		WebhookConfigured: boolPtr(!providerRequiresWebhookConfig(integ.Provider)),
	}

	if err := h.db.Create(&conn).Error; err != nil {
		slog.Error("failed to create in-connection", "error", err, "org_id", org.ID, "user_id", user.ID, "integration_id", integ.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
		return
	}

	conn.InIntegration = integ
	slog.Info("in-connection created", "connection_id", conn.ID, "org_id", org.ID, "user_id", user.ID, "provider", integ.Provider)
	writeJSON(w, http.StatusCreated, h.toInConnectionResponse(conn))
}
