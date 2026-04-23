package handler

import (
	"net/http"


	"github.com/usehiveloop/hiveloop/internal/model"
)

// ListInIntegrations handles GET /admin/v1/in-integrations.
// @Summary List platform integrations
// @Description Returns app-owned (platform) integrations with cursor pagination (default limit 100, max 500).
// @Tags admin
// @Produce json
// @Param limit query int false "Page size (default 100, max 500)"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} map[string]any
// @Security BearerAuth
// @Router /admin/v1/in-integrations [get]
func (h *AdminHandler) ListInIntegrations(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePaginationLimits(r, 100, 500)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := applyPagination(h.db.Model(&model.InIntegration{}), cursor, limit)

	var integrations []model.InIntegration
	if err := q.Find(&integrations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list in-integrations"})
		return
	}

	hasMore := len(integrations) > limit
	if hasMore {
		integrations = integrations[:limit]
	}

	resp := make([]adminInIntegrationResponse, len(integrations))
	for i, integ := range integrations {
		resp[i] = toAdminInIntegrationResponse(integ)
	}

	body := map[string]any{"data": resp, "has_more": hasMore}
	if hasMore {
		last := integrations[len(integrations)-1]
		body["next_cursor"] = encodeCursor(last.CreatedAt, last.ID)
	}
	writeJSON(w, http.StatusOK, body)
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
	supported := h.catalog.ListProviders()
	supportedSet := make(map[string]struct{}, len(supported))
	for _, name := range supported {
		supportedSet[name] = struct{}{}
	}

	providers := h.nango.GetProviders()

	type providerInfo struct {
		Name                     string `json:"name"`
		DisplayName              string `json:"display_name"`
		AuthMode                 string `json:"auth_mode"`
		WebhookUserDefinedSecret bool   `json:"webhook_user_defined_secret,omitempty"`
	}

	resp := make([]providerInfo, 0, len(supported))
	for _, p := range providers {
		if _, ok := supportedSet[p.Name]; !ok {
			continue
		}
		resp = append(resp, providerInfo{
			Name:                     p.Name,
			DisplayName:              p.DisplayName,
			AuthMode:                 p.AuthMode,
			WebhookUserDefinedSecret: p.WebhookUserDefinedSecret,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}



