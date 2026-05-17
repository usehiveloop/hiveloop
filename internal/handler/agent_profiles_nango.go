package handler

import (
	"net/http"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
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
	CustomAppConfigured    bool                               `json:"custom_app_configured,omitempty"`
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
		var catalogDisplayName string
		if providerDef, ok := h.catalog.GetProvider(provider); ok && providerDef != nil {
			catalogDisplayName = providerDef.DisplayName
		}
		template, _ := h.nango.GetProviderTemplate(nangoProvider)
		cfg := parseNangoConfig(buildNangoConfig(nil, template, h.nango.CallbackURL()))
		if cfg == nil {
			cfg = &model.NangoConfig{AuthMode: nangoMeta.AuthMode, CallbackURL: h.nango.CallbackURL()}
		}
		cfg.WebhookSecret = ""
		cfg.WebhookURL = ""
		cfg.WebhookUserDefinedSecret = nangoMeta.WebhookUserDefinedSecret

		item := profileProviderAvailableResponse{
			Provider:        provider,
			DisplayName:     profileProviderDisplayName(provider, catalogDisplayName, nangoMeta.DisplayName),
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
				item.CustomAppConfigured = profileCustomAppConfigured(integ.Meta)
				item.ProviderConfigKey = &key
				if integCfg := parseNangoConfig(integ.NangoConfig); integCfg != nil {
					item.NangoConfig = integCfg
				}
			}
		}
		resp = append(resp, item)
	}

	writeJSON(w, http.StatusOK, resp)
}
