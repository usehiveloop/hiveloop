package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
	"github.com/ziraloop/ziraloop/internal/middleware"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/nango"
)

// IntegrationHandler manages integration CRUD operations.
type IntegrationHandler struct {
	db      *gorm.DB
	nango   *nango.Client
	catalog *catalog.Catalog
}

// NewIntegrationHandler creates a new integration handler.
func NewIntegrationHandler(db *gorm.DB, nangoClient *nango.Client, cat *catalog.Catalog) *IntegrationHandler {
	return &IntegrationHandler{db: db, nango: nangoClient, catalog: cat}
}

type createIntegrationRequest struct {
	Provider    string             `json:"provider"`
	DisplayName string             `json:"display_name"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

type updateIntegrationRequest struct {
	DisplayName *string            `json:"display_name,omitempty"`
	Credentials *nango.Credentials `json:"credentials,omitempty"`
	Meta        model.JSON         `json:"meta,omitempty"`
}

type integrationResponse struct {
	ID          string     `json:"id"`
	UniqueKey   string     `json:"unique_key"`
	Provider    string     `json:"provider"`
	DisplayName string     `json:"display_name"`
	Meta        model.JSON `json:"meta,omitempty"`
	NangoConfig model.JSON `json:"nango_config,omitempty"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

func toIntegrationResponse(integ model.Integration) integrationResponse {
	return integrationResponse{
		ID:          integ.ID.String(),
		UniqueKey:   integ.UniqueKey,
		Provider:    integ.Provider,
		DisplayName: integ.DisplayName,
		Meta:        integ.Meta,
		NangoConfig: integ.NangoConfig,
		CreatedAt:   integ.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   integ.UpdatedAt.Format(time.RFC3339),
	}
}

// buildNangoConfig assembles a curated, non-sensitive config blob from Nango
// integration response data and the cached provider template.
func buildNangoConfig(integResp map[string]any, template map[string]any, callbackURL string) model.JSON {
	config := model.JSON{}

	// Extract fields from the integration response (nested under "data")
	if data, ok := integResp["data"].(map[string]any); ok {
		for _, key := range []string{"logo", "webhook_url", "forward_webhooks"} {
			if v, exists := data[key]; exists {
				config[key] = v
			}
		}

		// Extract user-defined webhook_secret from credentials (for OAUTH/TBA providers)
		if creds, ok := data["credentials"].(map[string]any); ok {
			if ws, ok := creds["webhook_secret"].(string); ok && ws != "" {
				config["webhook_secret"] = ws
			}
		}
	}

	// Extract non-sensitive fields from provider template
	if template != nil {
		for _, key := range []string{
			"auth_mode", "authorization_url", "docs", "setup_guide_url",
			"docs_connect", "categories", "connection_config", "webhook_routing_script",
		} {
			if v, exists := template[key]; exists {
				config[key] = v
			}
		}
		// Rename "credentials" to "credentials_schema" (schema only, not actual secrets)
		if v, exists := template["credentials"]; exists {
			config["credentials_schema"] = v
		}
		// Extract webhook_user_defined_secret flag
		if v, exists := template["webhook_user_defined_secret"]; exists {
			config["webhook_user_defined_secret"] = v
		}
	}

	// Computed
	config["callback_url"] = callbackURL

	return config
}

// nangoKey returns the org-namespaced provider config key for Nango.
func nangoKey(orgID uuid.UUID, uniqueKey string) string {
	return fmt.Sprintf("%s_%s", orgID.String(), uniqueKey)
}

// validateCredentials validates the credentials object against the provider's auth_mode.
// Both create and update require all mandatory fields — Nango does not support partial
// credential updates.
func validateCredentials(provider nango.Provider, creds *nango.Credentials) error {
	mode := provider.AuthMode

	switch mode {
	case "OAUTH1", "OAUTH2", "TBA":
		if creds == nil {
			return fmt.Errorf("credentials required for %s auth mode", mode)
		}
		if creds.Type != mode {
			return fmt.Errorf("credentials.type must be %q for provider %q", mode, provider.Name)
		}
		if creds.ClientID == "" {
			return fmt.Errorf("client_id is required for %s auth mode", mode)
		}
		if creds.ClientSecret == "" {
			return fmt.Errorf("client_secret is required for %s auth mode", mode)
		}

	case "APP":
		if creds == nil {
			return fmt.Errorf("credentials required for APP auth mode")
		}
		if creds.Type != "APP" {
			return fmt.Errorf("credentials.type must be \"APP\" for provider %q", provider.Name)
		}
		if creds.AppID == "" {
			return fmt.Errorf("app_id is required for APP auth mode")
		}
		if creds.AppLink == "" {
			return fmt.Errorf("app_link is required for APP auth mode")
		}
		if creds.PrivateKey == "" {
			return fmt.Errorf("private_key is required for APP auth mode")
		}

	case "CUSTOM":
		if creds == nil {
			return fmt.Errorf("credentials required for CUSTOM auth mode")
		}
		if creds.Type != "CUSTOM" {
			return fmt.Errorf("credentials.type must be \"CUSTOM\" for provider %q", provider.Name)
		}
		if creds.ClientID == "" || creds.ClientSecret == "" || creds.AppID == "" || creds.AppLink == "" || creds.PrivateKey == "" {
			return fmt.Errorf("client_id, client_secret, app_id, app_link, and private_key are all required for CUSTOM auth mode")
		}

	case "MCP_OAUTH2":
		if creds == nil {
			return fmt.Errorf("credentials required for MCP_OAUTH2 auth mode")
		}
		if creds.Type != "MCP_OAUTH2" {
			return fmt.Errorf("credentials.type must be \"MCP_OAUTH2\" for provider %q", provider.Name)
		}
		if provider.ClientRegistration == "static" {
			if creds.ClientID == "" {
				return fmt.Errorf("client_id is required for MCP_OAUTH2 with static client registration")
			}
			if creds.ClientSecret == "" {
				return fmt.Errorf("client_secret is required for MCP_OAUTH2 with static client registration")
			}
		}

	case "MCP_OAUTH2_GENERIC":
		// Credentials are optional for this mode
		if creds != nil && creds.Type != "MCP_OAUTH2_GENERIC" {
			return fmt.Errorf("credentials.type must be \"MCP_OAUTH2_GENERIC\" for provider %q", provider.Name)
		}

	case "INSTALL_PLUGIN":
		if creds == nil {
			return fmt.Errorf("credentials required for INSTALL_PLUGIN auth mode")
		}
		if creds.Type != "INSTALL_PLUGIN" {
			return fmt.Errorf("credentials.type must be \"INSTALL_PLUGIN\" for provider %q", provider.Name)
		}
		if creds.AppLink == "" {
			return fmt.Errorf("app_link is required for INSTALL_PLUGIN auth mode")
		}

	case "BASIC", "API_KEY", "NONE", "OAUTH2_CC", "JWT", "BILL", "TWO_STEP", "SIGNATURE", "APP_STORE":
		if creds != nil {
			return fmt.Errorf("credentials must not be provided for %s auth mode", mode)
		}

	default:
		if creds != nil {
			return fmt.Errorf("credentials must not be provided for unknown auth mode %q", mode)
		}
	}

	return nil
}

// Create handles POST /v1/integrations.
//
// @Summary Create an integration
// @Description Creates a new integration backed by a Nango provider.
// @Tags integrations
// @Accept json
// @Produce json
// @Param body body createIntegrationRequest true "Integration parameters"
// @Success 201 {object} integrationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/integrations [post]
func (h *IntegrationHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return
	}
	if req.DisplayName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "display_name is required"})
		return
	}

	// Validate provider exists in Nango catalog
	provider, found := h.nango.GetProvider(req.Provider)
	if !found {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider %q", req.Provider)})
		return
	}

	// Validate provider has verified action definitions in the catalog.
	if _, ok := h.catalog.GetProvider(req.Provider); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %q is not supported — no action definitions available", req.Provider)})
		return
	}

	// Validate credentials against provider's auth_mode
	if err := validateCredentials(provider, req.Credentials); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// Auto-generate unique_key (internal only, never exposed to users)
	integID := uuid.New()
	uniqueKey := fmt.Sprintf("%s-%s", req.Provider, integID.String()[:8])

	// Push to Nango first (source of truth for OAuth credentials)
	nk := nangoKey(org.ID, uniqueKey)
	nangoReq := nango.CreateIntegrationRequest{
		UniqueKey:   nk,
		Provider:    req.Provider,
		Credentials: req.Credentials,
	}
	slog.Info("creating nango integration", "org_id", org.ID, "provider", req.Provider, "nango_key", nk)
	if err := h.nango.CreateIntegration(r.Context(), nangoReq); err != nil {
		slog.Error("nango integration creation failed", "error", err, "org_id", org.ID, "provider", req.Provider)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create integration in Nango: " + err.Error()})
		return
	}
	slog.Info("nango integration created", "org_id", org.ID, "provider", req.Provider, "nango_key", nk)

	// Fetch Nango config (best-effort — don't fail create if this errors)
	var nangoConfig model.JSON
	integResp, err := h.nango.GetIntegration(r.Context(), nk)
	if err != nil {
		slog.Warn("failed to fetch nango integration details for config", "error", err, "nango_key", nk)
	} else {
		template, _ := h.nango.GetProviderTemplate(req.Provider)
		nangoConfig = buildNangoConfig(integResp, template, h.nango.CallbackURL())
	}

	integ := model.Integration{
		ID:          integID,
		OrgID:       org.ID,
		UniqueKey:   uniqueKey,
		Provider:    req.Provider,
		DisplayName: req.DisplayName,
		Meta:        req.Meta,
		NangoConfig: nangoConfig,
	}

	if err := h.db.Create(&integ).Error; err != nil {
		slog.Error("failed to store integration", "error", err, "org_id", org.ID, "provider", req.Provider)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create integration"})
		return
	}

	slog.Info("integration created", "org_id", org.ID, "integration_id", integ.ID, "provider", req.Provider, "display_name", req.DisplayName)
	writeJSON(w, http.StatusCreated, toIntegrationResponse(integ))
}

// Get handles GET /v1/integrations/{id}.
//
// @Summary Get an integration
// @Description Returns a single integration by ID.
// @Tags integrations
// @Produce json
// @Param id path string true "Integration ID"
// @Success 200 {object} integrationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/integrations/{id} [get]
func (h *IntegrationHandler) Get(w http.ResponseWriter, r *http.Request) {
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

	var integ model.Integration
	if err := h.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", integID, org.ID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get integration"})
		return
	}

	nk := nangoKey(org.ID, integ.UniqueKey)
	integResp, err := h.nango.GetIntegration(r.Context(), nk)
	if err != nil {
		slog.Warn("failed to fetch nango integration", "error", err, "integration_id", integ.ID)
	} else {
		template, _ := h.nango.GetProviderTemplate(integ.Provider)
		liveConfig := buildNangoConfig(integResp, template, h.nango.CallbackURL())

		if data, ok := integResp["data"].(map[string]any); ok {
			if creds, ok := data["credentials"].(map[string]any); ok {
				if pk, ok := creds["private_key"].(string); ok && pk != "" {
					if decoded, err := base64.StdEncoding.DecodeString(pk); err == nil {
						creds["private_key"] = string(decoded)
					}
				}
				liveConfig["credentials"] = creds
			}
		}

		integ.NangoConfig = liveConfig
	}

	writeJSON(w, http.StatusOK, toIntegrationResponse(integ))
}

// List handles GET /v1/integrations.
//
// @Summary List integrations
// @Description Returns integrations for the current organization with cursor pagination.
// @Tags integrations
// @Produce json
// @Param limit query int false "Max items per page (1-100, default 50)"
// @Param cursor query string false "Pagination cursor from previous response"
// @Param provider query string false "Filter by provider name"
// @Param meta query string false "Filter by JSONB meta (PostgreSQL @> operator)"
// @Success 200 {object} paginatedResponse[integrationResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/integrations [get]
func (h *IntegrationHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("org_id = ? AND deleted_at IS NULL", org.ID)

	if provider := r.URL.Query().Get("provider"); provider != "" {
		q = q.Where("provider = ?", provider)
	}
	if metaFilter := r.URL.Query().Get("meta"); metaFilter != "" {
		q = q.Where("meta @> ?::jsonb", metaFilter)
	}

	q = applyPagination(q, cursor, limit)

	var integrations []model.Integration
	if err := q.Find(&integrations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list integrations"})
		return
	}

	hasMore := len(integrations) > limit
	if hasMore {
		integrations = integrations[:limit]
	}

	resp := make([]integrationResponse, len(integrations))
	for i, integ := range integrations {
		resp[i] = toIntegrationResponse(integ)
	}

	result := paginatedResponse[integrationResponse]{
		Data:    resp,
		HasMore: hasMore,
	}
	if hasMore {
		last := integrations[len(integrations)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, result)
}

// Update handles PUT /v1/integrations/{id}.
//
// @Summary Update an integration
// @Description Updates an integration's display name, credentials, or metadata.
// @Tags integrations
// @Accept json
// @Produce json
// @Param id path string true "Integration ID"
// @Param body body updateIntegrationRequest true "Fields to update"
// @Success 200 {object} integrationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/integrations/{id} [put]
func (h *IntegrationHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	var integ model.Integration
	if err := h.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", integID, org.ID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}

	var req updateIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Credentials != nil {
		provider, found := h.nango.GetProvider(integ.Provider)
		if !found {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider %q", integ.Provider)})
			return
		}
		if err := validateCredentials(provider, req.Credentials); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		nk := nangoKey(org.ID, integ.UniqueKey)
		nangoReq := nango.UpdateIntegrationRequest{
			Credentials: req.Credentials,
		}
		slog.Info("updating nango integration credentials", "org_id", org.ID, "integration_id", integ.ID, "nango_key", nk)
		if err := h.nango.UpdateIntegration(r.Context(), nk, nangoReq); err != nil {
			slog.Error("nango integration update failed", "error", err, "org_id", org.ID, "integration_id", integ.ID)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to update integration in Nango: " + err.Error()})
			return
		}
		slog.Info("nango integration credentials updated", "org_id", org.ID, "integration_id", integ.ID)

		integResp, fetchErr := h.nango.GetIntegration(r.Context(), nk)
		if fetchErr != nil {
			slog.Warn("failed to fetch nango integration details for config rebuild", "error", fetchErr, "nango_key", nk)
		} else {
			template, _ := h.nango.GetProviderTemplate(integ.Provider)
			integ.NangoConfig = buildNangoConfig(integResp, template, h.nango.CallbackURL())
		}

	}

	updates := map[string]any{}
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.Meta != nil {
		updates["meta"] = req.Meta
	}
	if integ.NangoConfig != nil {
		updates["nango_config"] = integ.NangoConfig
	}
	if len(updates) > 0 {
		if err := h.db.Model(&integ).Updates(updates).Error; err != nil {
			slog.Error("failed to update integration", "error", err, "org_id", org.ID, "integration_id", integ.ID)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update integration"})
			return
		}
	}

	h.db.Where("id = ?", integ.ID).First(&integ)

	slog.Info("integration updated", "org_id", org.ID, "integration_id", integ.ID, "credentials_updated", req.Credentials != nil)
	writeJSON(w, http.StatusOK, toIntegrationResponse(integ))
}

// Delete handles DELETE /v1/integrations/{id}.
//
// @Summary Delete an integration
// @Description Soft-deletes an integration and removes it from Nango.
// @Tags integrations
// @Produce json
// @Param id path string true "Integration ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/integrations/{id} [delete]
func (h *IntegrationHandler) Delete(w http.ResponseWriter, r *http.Request) {
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

	var integ model.Integration
	if err := h.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", integID, org.ID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}

	nk := nangoKey(org.ID, integ.UniqueKey)
	slog.Info("deleting nango integration", "org_id", org.ID, "integration_id", integ.ID, "nango_key", nk)
	if err := h.nango.DeleteIntegration(r.Context(), nk); err != nil {
		slog.Error("nango integration deletion failed", "error", err, "org_id", org.ID, "integration_id", integ.ID)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to delete integration from Nango: " + err.Error()})
		return
	}
	slog.Info("nango integration deleted", "org_id", org.ID, "integration_id", integ.ID)

	now := time.Now()
	if err := h.db.Model(&integ).Update("deleted_at", now).Error; err != nil {
		slog.Error("failed to soft-delete integration", "error", err, "org_id", org.ID, "integration_id", integ.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete integration"})
		return
	}

	slog.Info("integration deleted", "org_id", org.ID, "integration_id", integ.ID, "provider", integ.Provider)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type integrationProviderInfo struct {
	Name                     string `json:"name"`
	DisplayName              string `json:"display_name"`
	AuthMode                 string `json:"auth_mode"`
	WebhookUserDefinedSecret bool   `json:"webhook_user_defined_secret,omitempty"`
}

// ListProviders handles GET /v1/integrations/providers.
//
// @Summary List available providers
// @Description Returns Nango providers that have verified action definitions in the catalog.
// @Tags integrations
// @Produce json
// @Success 200 {array} integrationProviderInfo
// @Security BearerAuth
// @Router /v1/integrations/providers [get]
// @Router /v1/widget/integrations/providers [get]
func (h *IntegrationHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	// Only expose providers that have verified action definitions in the catalog.
	supported := h.catalog.ListProviders()
	supportedSet := make(map[string]struct{}, len(supported))
	for _, name := range supported {
		supportedSet[name] = struct{}{}
	}

	providers := h.nango.GetProviders()
	resp := make([]integrationProviderInfo, 0, len(supported))
	for _, p := range providers {
		if _, ok := supportedSet[p.Name]; !ok {
			continue
		}
		resp = append(resp, integrationProviderInfo{
			Name:                     p.Name,
			DisplayName:              p.DisplayName,
			AuthMode:                 p.AuthMode,
			WebhookUserDefinedSecret: p.WebhookUserDefinedSecret,
		})
	}
	slog.Info("listed integration providers", "total_nango", len(providers), "supported", len(resp))
	writeJSON(w, http.StatusOK, resp)
}
