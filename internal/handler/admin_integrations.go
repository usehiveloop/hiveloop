package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/nango"
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
	if err := h.db.Order("created_at DESC").Find(&integrations).Error; err != nil {
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

type adminCreateInIntegrationRequest struct {
	Provider    string             `json:"provider"`
	DisplayName string             `json:"display_name"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

type adminInIntegrationResponse struct {
	ID          string     `json:"id"`
	UniqueKey   string     `json:"unique_key"`
	Provider    string     `json:"provider"`
	DisplayName string     `json:"display_name"`
	Meta        model.JSON `json:"meta,omitempty"`
	NangoConfig model.JSON `json:"nango_config,omitempty"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

func toAdminInIntegrationResponse(i model.InIntegration) adminInIntegrationResponse {
	return adminInIntegrationResponse{
		ID:          i.ID.String(),
		UniqueKey:   i.UniqueKey,
		Provider:    i.Provider,
		DisplayName: i.DisplayName,
		Meta:        i.Meta,
		NangoConfig: i.NangoConfig,
		CreatedAt:   i.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   i.UpdatedAt.Format(time.RFC3339),
	}
}