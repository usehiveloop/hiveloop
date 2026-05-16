package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *AgentProfileHandler) resolveProfileIntegration(w http.ResponseWriter, r *http.Request) (model.Agent, uuid.UUID, model.InIntegration, bool) {
	agent, orgID, err := h.resolveEmployeeAgent(r)
	if err != nil {
		writeAgentProfileResolveError(w, err)
		return model.Agent{}, uuid.Nil, model.InIntegration{}, false
	}
	if h.nango == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "nango is not configured"})
		return model.Agent{}, uuid.Nil, model.InIntegration{}, false
	}
	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	if provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return model.Agent{}, uuid.Nil, model.InIntegration{}, false
	}
	if integrationEmployeeProfileCapability(provider) == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %q is not an employee profile provider", provider)})
		return model.Agent{}, uuid.Nil, model.InIntegration{}, false
	}
	capability := h.catalog.EmployeeProfileCapability(provider)
	var integ model.InIntegration
	query := h.db.Where("provider = ? AND deleted_at IS NULL", provider)
	if capability != nil && capability.CustomApp {
		query = query.Where("org_id = ? AND agent_id = ? AND custom_app = true", orgID, agent.ID)
	} else {
		query = query.Where("custom_app = false")
	}
	if err := query.First(&integ).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile integration not found"})
			return model.Agent{}, uuid.Nil, model.InIntegration{}, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load profile integration"})
		return model.Agent{}, uuid.Nil, model.InIntegration{}, false
	}
	return agent, orgID, integ, true
}

func (h *AgentProfileHandler) loadProfileCustomAppIntegration(ctx context.Context, orgID uuid.UUID, agentID uuid.UUID, provider string) (model.InIntegration, bool) {
	var integ model.InIntegration
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND provider = ? AND custom_app = true AND deleted_at IS NULL", orgID, agentID, provider).
		First(&integ).Error; err != nil {
		return model.InIntegration{}, false
	}
	return integ, true
}

func profileProviderDisplayName(provider string, fallback string) string {
	switch provider {
	case "linear-profile":
		return "Linear Profile"
	}
	if fallback != "" {
		return fallback
	}
	return provider
}

func (h *AgentProfileHandler) completeProfileFromConnection(w http.ResponseWriter, r *http.Request, agent model.Agent, orgID uuid.UUID, conn model.InConnection, label string) (agentProfileResponse, int, bool) {
	switch conn.InIntegration.Provider {
	case githubProfileProvider:
		return h.createGitHubProfileFromConnection(w, r, agent, orgID, conn, label)
	default:
		return h.createGenericNangoProfileFromConnection(w, r, agent, orgID, conn, label)
	}
}

func (h *AgentProfileHandler) createGenericNangoProfileFromConnection(w http.ResponseWriter, r *http.Request, agent model.Agent, orgID uuid.UUID, conn model.InConnection, requestedLabel string) (agentProfileResponse, int, bool) {
	now := time.Now().UTC()
	provider := conn.InIntegration.Provider
	label := strings.TrimSpace(requestedLabel)
	if label == "" {
		label = conn.InIntegration.DisplayName
	}
	if label == "" {
		label = provider
	}
	config := model.JSON{
		"in_connection_id":    conn.ID.String(),
		"nango_connection_id": conn.NangoConnectionID,
		"provider_config_key": inNangoKey(conn.InIntegration.UniqueKey),
	}
	if conn.InIntegration.CustomApp {
		config["custom_app_integration_id"] = conn.InIntegration.ID.String()
	}

	var profile model.AgentProfile
	err := h.db.Where(
		"agent_id = ? AND provider = ? AND deleted_at IS NULL AND revoked_at IS NULL",
		agent.ID, provider,
	).First(&profile).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load profile"})
		return agentProfileResponse{}, 0, false
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		profile = model.AgentProfile{
			ID:             uuid.New(),
			OrgID:          orgID,
			AgentID:        agent.ID,
			Provider:       provider,
			ExternalID:     conn.NangoConnectionID,
			Label:          label,
			Identity:       model.JSON{},
			Config:         config,
			Status:         "active",
			LastVerifiedAt: &now,
		}
		if err := h.db.Create(&profile).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create profile"})
			return agentProfileResponse{}, 0, false
		}
		return toAgentProfileResponse(profile), http.StatusCreated, true
	}

	profile.ExternalID = conn.NangoConnectionID
	profile.Label = label
	profile.Config = config
	profile.Status = "active"
	profile.StatusReason = ""
	profile.LastVerifiedAt = &now
	if err := h.db.Save(&profile).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update profile"})
		return agentProfileResponse{}, 0, false
	}
	return toAgentProfileResponse(profile), http.StatusOK, true
}

func (h *AgentProfileHandler) resolveProfileConnection(w http.ResponseWriter, r *http.Request, agent model.Agent, orgID uuid.UUID, provider string) (model.AgentProfile, model.InConnection, bool) {
	var profile model.AgentProfile
	err := h.db.Where(
		"agent_id = ? AND provider = ? AND deleted_at IS NULL AND revoked_at IS NULL",
		agent.ID, provider,
	).First(&profile).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not connected"})
			return model.AgentProfile{}, model.InConnection{}, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load profile"})
		return model.AgentProfile{}, model.InConnection{}, false
	}

	connectionID, err := uuid.Parse(stringFromJSON(profile.Config, "in_connection_id"))
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "profile is missing its connection"})
		return model.AgentProfile{}, model.InConnection{}, false
	}
	var conn model.InConnection
	if err := h.db.Preload("InIntegration").
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connectionID, orgID).
		First(&conn).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile connection not found"})
			return model.AgentProfile{}, model.InConnection{}, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load profile connection"})
		return model.AgentProfile{}, model.InConnection{}, false
	}
	if conn.InIntegration.Provider != provider {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "profile connection provider mismatch"})
		return model.AgentProfile{}, model.InConnection{}, false
	}
	return profile, conn, true
}

func profileConnectionResponse(conn model.InConnection) inConnectionResponse {
	return inConnectionResponse{
		ID:                conn.ID.String(),
		OrgID:             conn.OrgID.String(),
		InIntegrationID:   conn.InIntegrationID.String(),
		Provider:          conn.InIntegration.Provider,
		DisplayName:       conn.InIntegration.DisplayName,
		NangoConnectionID: conn.NangoConnectionID,
		Meta:              conn.Meta,
		EmployeeProfile:   integrationEmployeeProfileCapability(conn.InIntegration.Provider),
		WebhookConfigured: derefBool(conn.WebhookConfigured, true),
		CreatedAt:         conn.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         conn.UpdatedAt.Format(time.RFC3339),
	}
}
