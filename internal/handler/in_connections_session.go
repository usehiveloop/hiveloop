package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

// @Summary Create a connect session
// @Description Creates a Nango connect session for the authenticated user to initiate OAuth.
// @Tags in-connections
// @Produce json
// @Param id path string true "Integration ID"
// @Success 201 {object} inConnectSessionResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/in/integrations/{id}/connect-session [post]
func (h *InConnectionHandler) CreateConnectSession(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

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
	nangoReq := nango.CreateConnectSessionRequest{
		EndUser: nango.ConnectSessionEndUser{
			ID: user.ID.String(),
		},
		AllowedIntegrations: []string{nk},
	}

	sess, err := h.nango.CreateConnectSession(r.Context(), nangoReq)
	if err != nil {
		slog.Error("nango connect session creation failed", "error", err, "integration_id", integ.ID, "user_id", user.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create connect session: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, inConnectSessionResponse{
		Token:             sess.Token,
		ProviderConfigKey: nk,
	})
}

// @Summary Create a reconnect session for an existing connection
// @Description Creates a Nango connect session scoped to an existing connection, allowing OAuth re-authorization without creating a duplicate.
// @Tags in-connections
// @Produce json
// @Param id path string true "Connection ID"
// @Success 201 {object} inConnectSessionResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/in/connections/{id}/reconnect-session [post]
func (h *InConnectionHandler) CreateReconnectSession(w http.ResponseWriter, r *http.Request) {
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

	connID := chi.URLParam(r, "id")
	if connID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection id required"})
		return
	}

	var conn model.InConnection
	if err := h.db.Preload("InIntegration").Where("id = ? AND org_id = ? AND revoked_at IS NULL", connID, org.ID).First(&conn).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find connection"})
		return
	}

	nk := inNangoKey(conn.InIntegration.UniqueKey)

	sess, err := h.nango.CreateReconnectSession(r.Context(), nango.CreateReconnectSessionRequest{
		ConnectionID:  conn.NangoConnectionID,
		IntegrationID: nk,
	})
	if err != nil {
		slog.Error("nango reconnect session creation failed", "error", err, "connection_id", conn.ID, "user_id", user.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create reconnect session: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, inConnectSessionResponse{
		Token:             sess.Token,
		ProviderConfigKey: nk,
	})
}
