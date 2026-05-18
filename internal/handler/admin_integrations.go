package handler

import (
	"net/http"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// ListInIntegrations handles GET /admin/v1/in-integrations.
// @Summary List platform integrations
// @Description Returns all app-owned (platform) integrations.
// @Tags admin
// @Produce json
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/in-integrations [get]
func (h *AdminHandler) ListInIntegrations(w http.ResponseWriter, r *http.Request) {
	var integrations []model.InIntegration
	if err := h.db.Where("custom_app = false").Order("created_at DESC").Find(&integrations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list in-integrations"})
		return
	}

	resp := make([]adminInIntegrationResponse, len(integrations))
	for i, integ := range integrations {
		resp[i] = toAdminInIntegrationResponse(integ)
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// ListInConnections handles GET /admin/v1/in-connections.
// @Summary List user connections to platform integrations
// @Description Returns all user connections to app-owned integrations.
// @Tags admin
// @Produce json
// @Param user_id query string false "Filter by user ID"
// @Param revoked query string false "Filter by revoked status (true/false)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/in-connections [get]
func (h *AdminHandler) ListInConnections(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Model(&model.InConnection{})
	if userID := r.URL.Query().Get("user_id"); userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if r.URL.Query().Get("revoked") == "true" {
		q = q.Where("revoked_at IS NOT NULL")
	} else if r.URL.Query().Get("revoked") == "false" {
		q = q.Where("revoked_at IS NULL")
	}

	q = applyPagination(q, cursor, limit)

	var connections []model.InConnection
	if err := q.Find(&connections).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list in-connections"})
		return
	}

	hasMore := len(connections) > limit
	if hasMore {
		connections = connections[:limit]
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": connections, "has_more": hasMore})
}

// ListInIntegrationProviders handles GET /admin/v1/in-integration-providers.
// @Summary List available integration providers
// @Description Returns providers supported for platform integrations (filtered by action catalog).
// @Tags admin
// @Produce json
// @Success 200 {array} map[string]any
// @Security BearerAuth
// @Router /admin/v1/in-integration-providers [get]
func (h *AdminHandler) ListInIntegrationProviders(w http.ResponseWriter, r *http.Request) {
	type providerInfo struct {
		Name                     string `json:"name"`
		DisplayName              string `json:"display_name"`
		AuthMode                 string `json:"auth_mode"`
		WebhookUserDefinedSecret bool   `json:"webhook_user_defined_secret,omitempty"`
	}

	displayNameOverride := map[string]string{
		"github-app-code-reviews": "GitHub App (Code Reviews)",
		"linear-profile":          "Linear Profile",
		"notion-profile":          "Notion Profile",
	}

	resp := make([]providerInfo, 0)
	for _, name := range h.catalog.ListProviders() {
		nangoName := nangoProviderName(name)
		p, ok := h.nango.GetProvider(nangoName)
		if !ok {
			continue
		}
		display := p.DisplayName
		if override, ok := displayNameOverride[name]; ok {
			display = override
		}
		resp = append(resp, providerInfo{
			Name:                     name,
			DisplayName:              display,
			AuthMode:                 p.AuthMode,
			WebhookUserDefinedSecret: p.WebhookUserDefinedSecret,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}
