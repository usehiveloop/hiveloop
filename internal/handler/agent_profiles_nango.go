package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

type completeAgentProfileRequest struct {
	NangoConnectionID string     `json:"nango_connection_id"`
	Label             string     `json:"label,omitempty"`
	Meta              model.JSON `json:"meta,omitempty"`
}

type completeAgentProfileResponse struct {
	Profile    agentProfileResponse `json:"profile"`
	Connection inConnectionResponse `json:"connection"`
}

type profileProviderAvailableResponse struct {
	Provider               string                             `json:"provider"`
	DisplayName            string                             `json:"display_name"`
	EmployeeProfile        *catalog.EmployeeProfileCapability `json:"employee_profile"`
	NangoConfig            *model.NangoConfig                 `json:"nango_config,omitempty"`
	Profile                *agentProfileResponse              `json:"profile,omitempty"`
	CustomAppIntegrationID *string                            `json:"custom_app_integration_id,omitempty"`
	ProviderConfigKey      *string                            `json:"provider_config_key,omitempty"`
}

type profileCustomAppRequest struct {
	DisplayName string             `json:"display_name,omitempty"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

type profileCustomAppResponse struct {
	Integration       inIntegrationResponse `json:"integration"`
	ProviderConfigKey string                `json:"provider_config_key"`
}

type updateProfileCustomAppRequest struct {
	DisplayName string             `json:"display_name,omitempty"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

// ListAvailableProfiles handles GET /v1/agents/{agentID}/profiles/available.
// @Summary List available employee profile providers
// @Description Returns profile-capable providers for an employee, including dynamic custom app setup metadata.
// @Tags agent-profiles
// @Produce json
// @Param agentID path string true "Agent ID (must be an AI employee)"
// @Success 200 {array} profileProviderAvailableResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/profiles/available [get]
func (h *AgentProfileHandler) ListAvailableProfiles(w http.ResponseWriter, r *http.Request) {
	agent, orgID, err := h.resolveEmployeeAgent(r)
	if err != nil {
		writeAgentProfileResolveError(w, err)
		return
	}
	if h.nango == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "nango is not configured"})
		return
	}

	var profiles []model.AgentProfile
	if err := h.db.Where("agent_id = ? AND deleted_at IS NULL AND revoked_at IS NULL", agent.ID).Find(&profiles).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load profiles"})
		return
	}
	profilesByProvider := make(map[string]model.AgentProfile, len(profiles))
	for _, profile := range profiles {
		profilesByProvider[profile.Provider] = profile
	}

	resp := make([]profileProviderAvailableResponse, 0)
	for _, provider := range h.catalog.ListProviders() {
		capability := h.catalog.EmployeeProfileCapability(provider)
		if capability == nil {
			continue
		}
		nangoProvider := nangoProviderName(provider)
		nangoMeta, ok := h.nango.GetProvider(nangoProvider)
		if !ok {
			continue
		}
		template, _ := h.nango.GetProviderTemplate(nangoProvider)
		cfg := parseNangoConfig(buildNangoConfig(nil, template, h.nango.CallbackURL()))
		if cfg == nil {
			cfg = &model.NangoConfig{AuthMode: nangoMeta.AuthMode, CallbackURL: h.nango.CallbackURL()}
		}
		cfg.WebhookSecret = ""
		cfg.WebhookURL = ""
		cfg.WebhookRoutingScript = ""
		cfg.WebhookUserDefinedSecret = nangoMeta.WebhookUserDefinedSecret

		item := profileProviderAvailableResponse{
			Provider:        provider,
			DisplayName:     profileProviderDisplayName(provider, nangoMeta.DisplayName),
			EmployeeProfile: capability,
			NangoConfig:     cfg,
		}
		if profile, ok := profilesByProvider[provider]; ok {
			profileResp := toAgentProfileResponse(profile)
			item.Profile = &profileResp
		}
		if capability.CustomApp {
			if integ, ok := h.loadProfileCustomAppIntegration(r.Context(), orgID, agent.ID, provider); ok {
				id := integ.ID.String()
				key := inNangoKey(integ.UniqueKey)
				item.CustomAppIntegrationID = &id
				item.ProviderConfigKey = &key
			}
		}
		resp = append(resp, item)
	}

	writeJSON(w, http.StatusOK, resp)
}

// CreateProfileCustomApp handles POST /v1/agents/{agentID}/profiles/{provider}/custom-app.
// @Summary Create a custom app integration for an employee profile
// @Description Creates one employee-scoped placeholder Nango integration for a custom-app profile provider.
// @Tags agent-profiles
// @Accept json
// @Produce json
// @Param agentID path string true "Agent ID (must be an AI employee)"
// @Param provider path string true "Profile provider"
// @Param body body profileCustomAppRequest false "Optional custom app metadata"
// @Success 201 {object} profileCustomAppResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/profiles/{provider}/custom-app [post]
func (h *AgentProfileHandler) CreateProfileCustomApp(w http.ResponseWriter, r *http.Request) {
	agent, orgID, err := h.resolveEmployeeAgent(r)
	if err != nil {
		writeAgentProfileResolveError(w, err)
		return
	}
	if h.nango == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "nango is not configured"})
		return
	}
	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	capability := h.catalog.EmployeeProfileCapability(provider)
	if capability == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %q is not an employee profile provider", provider)})
		return
	}
	if !capability.CustomApp {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %q does not require a custom app", provider)})
		return
	}

	var req profileCustomAppRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
	}

	nangoProvider := nangoProviderName(provider)
	providerMeta, ok := h.nango.GetProvider(nangoProvider)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unsupported provider %q", provider)})
		return
	}

	if integ, ok := h.loadProfileCustomAppIntegration(r.Context(), orgID, agent.ID, provider); ok {
		writeJSON(w, http.StatusOK, profileCustomAppResponse{
			Integration:       toInIntegrationResponse(integ),
			ProviderConfigKey: inNangoKey(integ.UniqueKey),
		})
		return
	}

	integID := uuid.New()
	uniqueKey := fmt.Sprintf("%s-%s-%s", provider, agent.ID.String()[:8], integID.String()[:8])
	nangoKey := inNangoKey(uniqueKey)
	nangoReq := nango.CreateIntegrationRequest{
		UniqueKey:   nangoKey,
		Provider:    nangoProvider,
		DisplayName: strings.TrimSpace(req.DisplayName),
		Credentials: profileCustomAppCredentialsWithScopes(providerMeta.AuthMode, capability.Scopes, req.Credentials),
	}
	if nangoReq.DisplayName == "" {
		nangoReq.DisplayName = profileProviderDisplayName(provider, providerMeta.DisplayName)
	}
	if err := h.nango.CreateIntegration(r.Context(), nangoReq); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "profile custom app nango integration creation failed",
			"error", err, "org_id", orgID, "agent_id", agent.ID, "provider", provider)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create custom app integration: " + err.Error()})
		return
	}

	nangoConfig := h.profileCustomAppNangoConfig(r.Context(), nangoProvider, nangoKey)
	integ := model.InIntegration{
		ID:          integID,
		UniqueKey:   uniqueKey,
		Provider:    provider,
		DisplayName: nangoReq.DisplayName,
		OrgID:       &orgID,
		AgentID:     &agent.ID,
		CustomApp:   true,
		Meta:        req.Meta,
		NangoConfig: nangoConfig,
	}
	if err := h.db.Create(&integ).Error; err != nil {
		_ = h.nango.DeleteIntegration(r.Context(), nangoKey)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store custom app integration"})
		return
	}

	writeJSON(w, http.StatusCreated, profileCustomAppResponse{
		Integration:       toInIntegrationResponse(integ),
		ProviderConfigKey: nangoKey,
	})
}

// UpdateProfileCustomApp handles PUT /v1/agents/{agentID}/profiles/{provider}/custom-app.
// @Summary Update a custom app integration for an employee profile
// @Description Updates the employee-scoped Nango integration credentials after the user has created the provider app with the placeholder webhook values.
// @Tags agent-profiles
// @Accept json
// @Produce json
// @Param agentID path string true "Agent ID (must be an AI employee)"
// @Param provider path string true "Profile provider"
// @Param body body updateProfileCustomAppRequest true "Custom app credentials"
// @Success 200 {object} profileCustomAppResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/agents/{agentID}/profiles/{provider}/custom-app [put]
func (h *AgentProfileHandler) UpdateProfileCustomApp(w http.ResponseWriter, r *http.Request) {
	agent, orgID, err := h.resolveEmployeeAgent(r)
	if err != nil {
		writeAgentProfileResolveError(w, err)
		return
	}
	if h.nango == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "nango is not configured"})
		return
	}
	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	capability := h.catalog.EmployeeProfileCapability(provider)
	if capability == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %q is not an employee profile provider", provider)})
		return
	}
	if !capability.CustomApp {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %q does not require a custom app", provider)})
		return
	}

	var req updateProfileCustomAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	nangoProvider := nangoProviderName(provider)
	providerMeta, ok := h.nango.GetProvider(nangoProvider)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unsupported provider %q", provider)})
		return
	}
	if err := validateCredentials(providerMeta, req.Credentials); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	applyProfileCustomAppScopes(req.Credentials, capability.Scopes)

	integ, ok := h.loadProfileCustomAppIntegration(r.Context(), orgID, agent.ID, provider)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "custom app integration not found"})
		return
	}
	if providerMeta.WebhookUserDefinedSecret && strings.TrimSpace(req.Credentials.WebhookSecret) == "" {
		if existingSecret := stringFromJSON(integ.NangoConfig, "webhook_secret"); existingSecret != "" {
			req.Credentials.WebhookSecret = existingSecret
		}
	}
	if providerMeta.WebhookUserDefinedSecret && strings.TrimSpace(req.Credentials.WebhookSecret) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "webhook_secret is required for this provider"})
		return
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = integ.DisplayName
	}
	nangoKey := inNangoKey(integ.UniqueKey)
	nangoReq := nango.UpdateIntegrationRequest{
		DisplayName: displayName,
		Credentials: req.Credentials,
	}
	if err := h.nango.UpdateIntegration(r.Context(), nangoKey, nangoReq); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "profile custom app nango integration update failed",
			"error", err, "org_id", orgID, "agent_id", agent.ID, "provider", provider, "integration_id", integ.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to update custom app integration: " + err.Error()})
		return
	}

	updates := map[string]any{
		"display_name": displayName,
		"nango_config": h.profileCustomAppNangoConfig(r.Context(), nangoProvider, nangoKey),
	}
	if req.Meta != nil {
		updates["meta"] = req.Meta
	}
	if err := h.db.Model(&integ).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store custom app integration"})
		return
	}
	if err := h.db.Where("id = ?", integ.ID).First(&integ).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reload custom app integration"})
		return
	}

	writeJSON(w, http.StatusOK, profileCustomAppResponse{
		Integration:       toInIntegrationResponse(integ),
		ProviderConfigKey: nangoKey,
	})
}

func profileCustomAppCredentialsWithScopes(authMode string, scopes []string, creds *nango.Credentials) *nango.Credentials {
	if len(scopes) == 0 {
		return creds
	}
	if creds == nil {
		creds = &nango.Credentials{Type: authMode}
	}
	applyProfileCustomAppScopes(creds, scopes)
	return creds
}

func applyProfileCustomAppScopes(creds *nango.Credentials, scopes []string) {
	if creds == nil || len(scopes) == 0 {
		return
	}
	creds.Scopes = strings.Join(scopes, ",")
}

func (h *AgentProfileHandler) profileCustomAppNangoConfig(ctx context.Context, nangoProvider string, nangoKey string) model.JSON {
	integResp, err := h.nango.GetIntegration(ctx, nangoKey)
	if err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "failed to fetch profile custom app integration details", "error", err, "nango_key", nangoKey)
		template, _ := h.nango.GetProviderTemplate(nangoProvider)
		return buildNangoConfig(nil, template, h.nango.CallbackURL())
	}
	template, _ := h.nango.GetProviderTemplate(nangoProvider)
	return buildNangoConfig(integResp, template, h.nango.CallbackURL())
}

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
