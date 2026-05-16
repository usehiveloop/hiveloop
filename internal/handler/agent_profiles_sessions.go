package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

// CreateProfileConnectSession handles POST /v1/agents/{agentID}/profiles/{provider}/connect-session.
// @Summary Create a profile connect session
// @Description Creates a Nango connect session for an employee profile provider.
// @Tags agent-profiles
// @Produce json
// @Param agentID path string true "Agent ID (must be an AI employee)"
// @Param provider path string true "Profile provider"
// @Success 201 {object} inConnectSessionResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/profiles/{provider}/connect-session [post]
func (h *AgentProfileHandler) CreateProfileConnectSession(w http.ResponseWriter, r *http.Request) {
	agent, orgID, integ, ok := h.resolveProfileIntegration(w, r)
	if !ok {
		return
	}
	if integ.CustomApp && !profileCustomAppConfigured(integ.Meta) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "custom app credentials must be saved before connecting this profile"})
		return
	}
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

	nk := inNangoKey(integ.UniqueKey)
	sess, err := h.nango.CreateConnectSession(r.Context(), nango.CreateConnectSessionRequest{
		EndUser: nango.ConnectSessionEndUser{
			ID: user.ID.String(),
		},
		AllowedIntegrations: []string{nk},
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "profile nango connect session creation failed",
			"error", err, "org_id", orgID, "agent_id", agent.ID, "provider", integ.Provider, "integration_id", integ.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create profile connect session: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, inConnectSessionResponse{
		Token:             sess.Token,
		ProviderConfigKey: nk,
	})
}

// CompleteProfileConnection handles POST /v1/agents/{agentID}/profiles/{provider}/complete.
// @Summary Complete a profile connection
// @Description Stores the Nango connection and attaches it as an employee profile.
// @Tags agent-profiles
// @Accept json
// @Produce json
// @Param agentID path string true "Agent ID (must be an AI employee)"
// @Param provider path string true "Profile provider"
// @Param body body completeAgentProfileRequest true "Nango connection"
// @Success 201 {object} completeAgentProfileResponse
// @Success 200 {object} completeAgentProfileResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/profiles/{provider}/complete [post]
func (h *AgentProfileHandler) CompleteProfileConnection(w http.ResponseWriter, r *http.Request) {
	agent, orgID, integ, ok := h.resolveProfileIntegration(w, r)
	if !ok {
		return
	}
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}
	var req completeAgentProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.NangoConnectionID = strings.TrimSpace(req.NangoConnectionID)
	if req.NangoConnectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nango_connection_id is required"})
		return
	}

	conn, ok := h.upsertProfileBackingConnection(w, r, orgID, user.ID, integ, req.NangoConnectionID, req.Meta)
	if !ok {
		return
	}

	profile, status, ok := h.completeProfileFromConnection(w, r, agent, orgID, conn, req.Label)
	if !ok {
		return
	}
	writeJSON(w, status, completeAgentProfileResponse{
		Profile:    profile,
		Connection: profileConnectionResponse(conn),
	})
}

func (h *AgentProfileHandler) upsertProfileBackingConnection(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, userID uuid.UUID, integ model.InIntegration, nangoConnectionID string, meta model.JSON) (model.InConnection, bool) {
	webhookConfigured := boolPtr(!providerRequiresWebhookConfig(integ.Provider))
	var conn model.InConnection
	err := h.db.Preload("InIntegration").
		Where("org_id = ? AND in_integration_id = ? AND nango_connection_id = ? AND revoked_at IS NULL", orgID, integ.ID, nangoConnectionID).
		First(&conn).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load profile connection"})
		return model.InConnection{}, false
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		conn = model.InConnection{
			ID:                uuid.New(),
			OrgID:             orgID,
			UserID:            userID,
			InIntegrationID:   integ.ID,
			InIntegration:     integ,
			NangoConnectionID: nangoConnectionID,
			Meta:              meta,
			WebhookConfigured: webhookConfigured,
		}
		if err := h.db.Create(&conn).Error; err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to create profile backing connection",
				"error", err, "org_id", orgID, "provider", integ.Provider, "integration_id", integ.ID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create profile connection"})
			return model.InConnection{}, false
		}
		return conn, true
	}

	conn.UserID = userID
	conn.Meta = meta
	conn.WebhookConfigured = webhookConfigured
	if err := h.db.Save(&conn).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to update profile backing connection",
			"error", err, "connection_id", conn.ID, "org_id", orgID, "provider", integ.Provider)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update profile connection"})
		return model.InConnection{}, false
	}
	conn.InIntegration = integ
	return conn, true
}

// CreateProfileReconnectSession handles POST /v1/agents/{agentID}/profiles/{provider}/reconnect-session.
// @Summary Create a profile reconnect session
// @Description Creates a Nango reconnect session for the existing employee profile provider.
// @Tags agent-profiles
// @Produce json
// @Param agentID path string true "Agent ID (must be an AI employee)"
// @Param provider path string true "Profile provider"
// @Success 201 {object} inConnectSessionResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/profiles/{provider}/reconnect-session [post]
func (h *AgentProfileHandler) CreateProfileReconnectSession(w http.ResponseWriter, r *http.Request) {
	agent, orgID, integ, ok := h.resolveProfileIntegration(w, r)
	if !ok {
		return
	}
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}

	profile, conn, ok := h.resolveProfileConnection(w, r, agent, orgID, integ.Provider)
	if !ok {
		return
	}
	nk := inNangoKey(conn.InIntegration.UniqueKey)
	sess, err := h.nango.CreateReconnectSession(r.Context(), nango.CreateReconnectSessionRequest{
		ConnectionID:  conn.NangoConnectionID,
		IntegrationID: nk,
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "profile nango reconnect session creation failed",
			"error", err, "profile_id", profile.ID, "connection_id", conn.ID, "user_id", user.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create profile reconnect session: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, inConnectSessionResponse{
		Token:             sess.Token,
		ProviderConfigKey: nk,
	})
}
