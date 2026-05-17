package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
)

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
		nangoConfig, err := h.refreshedProfileCustomAppConfig(r.Context(), nangoProvider, inNangoKey(integ.UniqueKey), providerMeta.WebhookUserDefinedSecret)
		if err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "profile custom app config refresh failed",
				"error", err, "org_id", orgID, "agent_id", agent.ID, "provider", provider, "integration_id", integ.ID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to refresh custom app integration"})
			return
		}
		if err := h.db.Model(&integ).Update("nango_config", nangoConfig).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store custom app integration"})
			return
		}
		integ.NangoConfig = nangoConfig
		writeJSON(w, http.StatusOK, profileCustomAppResponse{
			Integration:       toInIntegrationResponse(integ),
			ProviderConfigKey: inNangoKey(integ.UniqueKey),
		})
		return
	}

	integID := uuid.New()
	uniqueKey := fmt.Sprintf("%s-%s-%s", provider, agent.ID.String()[:8], integID.String()[:8])
	nangoKey := inNangoKey(uniqueKey)
	webhookSecret := ""
	if providerMeta.WebhookUserDefinedSecret {
		var err error
		webhookSecret, err = randomHex(24)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate webhook secret"})
			return
		}
	}
	nangoReq := nango.CreateIntegrationRequest{
		UniqueKey:   nangoKey,
		Provider:    nangoProvider,
		DisplayName: strings.TrimSpace(req.DisplayName),
		Credentials: profileCustomAppPlaceholderCredentials(providerMeta.AuthMode, capability.Scopes, req.Credentials, webhookSecret),
	}
	if nangoReq.DisplayName == "" {
		var catalogDisplayName string
		if providerDef, ok := h.catalog.GetProvider(provider); ok && providerDef != nil {
			catalogDisplayName = providerDef.DisplayName
		}
		nangoReq.DisplayName = profileProviderDisplayName(provider, catalogDisplayName, providerMeta.DisplayName)
	}
	if err := h.nango.CreateIntegration(r.Context(), nangoReq); err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "profile custom app nango integration creation failed",
			"error", err, "org_id", orgID, "agent_id", agent.ID, "provider", provider)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create custom app integration: " + err.Error()})
		return
	}

	nangoConfig := h.profileCustomAppNangoConfig(r.Context(), nangoProvider, nangoKey)
	if webhookSecret != "" && stringFromJSON(nangoConfig, "webhook_secret") == "" {
		nangoConfig["webhook_secret"] = webhookSecret
	}
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
		"meta":         profileCustomAppConfiguredMeta(integ.Meta, req.Meta),
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
