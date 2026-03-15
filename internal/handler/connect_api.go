package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/crypto"
	"github.com/llmvault/llmvault/internal/mcp/catalog"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
	"github.com/llmvault/llmvault/internal/nango"
	"github.com/llmvault/llmvault/internal/proxy"
	"github.com/llmvault/llmvault/internal/registry"
	"github.com/llmvault/llmvault/internal/resources"
)

// ConnectAPIHandler serves the Connect widget's API endpoints.
type ConnectAPIHandler struct {
	db        *gorm.DB
	kms       *crypto.KeyWrapper
	reg       *registry.Registry
	nango     *nango.Client
	catalog   *catalog.Catalog
	discovery *resources.Discovery
}

// NewConnectAPIHandler creates a new connect API handler.
func NewConnectAPIHandler(db *gorm.DB, kms *crypto.KeyWrapper, reg *registry.Registry, nangoClient *nango.Client, cat *catalog.Catalog) *ConnectAPIHandler {
	discovery := resources.NewDiscovery(cat, nangoClient)
	return &ConnectAPIHandler{db: db, kms: kms, reg: reg, nango: nangoClient, catalog: cat, discovery: discovery}
}

// knownBaseURLs provides base URLs for providers that lack an API field in the registry.
var knownBaseURLs = map[string]string{
	"openai":       "https://api.openai.com",
	"anthropic":    "https://api.anthropic.com",
	"google":       "https://generativelanguage.googleapis.com",
	"groq":         "https://api.groq.com/openai",
	"mistral":      "https://api.mistral.ai",
	"cohere":       "https://api.cohere.com",
	"fireworks-ai": "https://api.fireworks.ai/inference",
	"togetherai":   "https://api.together.xyz",
	"perplexity":   "https://api.perplexity.ai",
	"xai":          "https://api.x.ai",
	"deepinfra":    "https://api.deepinfra.com",
	"upstage":      "https://api.upstage.ai",
	"friendli":     "https://api.friendli.ai",
	"baseten":      "https://api.baseten.co",
	"nvidia":       "https://integrate.api.nvidia.com",
	"azure":        "https://models.inference.ai.azure.com",
	"cerebras":     "https://inference.cerebras.ai",
	"novita-ai":    "https://api.novita.ai",
	"huggingface":  "https://api-inference.huggingface.co",
}

// knownAuthSchemes overrides the default "bearer" for providers that use different schemes.
var knownAuthSchemes = map[string]string{
	"azure":  "api-key",
	"google": "query_param",
}

type connectionResponse struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	ProviderID   string `json:"provider_id,omitempty"`
	ProviderName string `json:"provider_name,omitempty"`
	BaseURL      string `json:"base_url"`
	AuthScheme   string `json:"auth_scheme"`
	CreatedAt    string `json:"created_at"`
}

type createConnectionRequest struct {
	ProviderID string `json:"provider_id"`
	APIKey     string `json:"api_key"`
	Label      string `json:"label,omitempty"`
}

type sessionInfoResponse struct {
	ID               string   `json:"id"`
	IdentityID       *string  `json:"identity_id,omitempty"`
	ExternalID       string   `json:"external_id,omitempty"`
	AllowedProviders []string `json:"allowed_providers,omitempty"`
	Permissions      []string `json:"permissions,omitempty"`
	ActivatedAt      *string  `json:"activated_at,omitempty"`
	ExpiresAt        string   `json:"expires_at"`
}

// SessionInfo handles GET /v1/widget/session.
// @Summary Get session info
// @Description Returns details about the current Connect widget session.
// @Tags widget
// @Produce json
// @Success 200 {object} sessionInfoResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/widget/session [get]
func (h *ConnectAPIHandler) SessionInfo(w http.ResponseWriter, r *http.Request) {
	sess, ok := middleware.ConnectSessionFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing session"})
		return
	}

	resp := sessionInfoResponse{
		ID:               sess.ID.String(),
		ExternalID:       sess.ExternalID,
		AllowedProviders: sess.AllowedProviders,
		Permissions:      sess.Permissions,
		ExpiresAt:        sess.ExpiresAt.Format(time.RFC3339),
	}
	if sess.IdentityID != nil {
		s := sess.IdentityID.String()
		resp.IdentityID = &s
	}
	if sess.ActivatedAt != nil {
		s := sess.ActivatedAt.Format(time.RFC3339)
		resp.ActivatedAt = &s
	}

	writeJSON(w, http.StatusOK, resp)
}

// ListProviders handles GET /v1/widget/providers.
// @Summary List providers for widget
// @Description Returns providers available to the current session, filtered by allowed_providers.
// @Tags widget
// @Produce json
// @Success 200 {array} providerSummary
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/widget/providers [get]
func (h *ConnectAPIHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	sess, ok := middleware.ConnectSessionFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing session"})
		return
	}

	allProviders := h.reg.AllProviders()

	// Filter by session's allowed_providers if set
	allowedSet := make(map[string]bool, len(sess.AllowedProviders))
	for _, p := range sess.AllowedProviders {
		allowedSet[p] = true
	}

	var result []providerSummary
	for _, p := range allProviders {
		if len(allowedSet) > 0 && !allowedSet[p.ID] {
			continue
		}
		result = append(result, providerSummary{
			ID:         p.ID,
			Name:       p.Name,
			API:        p.API,
			Doc:        p.Doc,
			ModelCount: len(p.Models),
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// ListConnections handles GET /v1/widget/connections.
// @Summary List connections
// @Description Returns active connections (credentials) for the session's identity with cursor pagination.
// @Tags widget
// @Produce json
// @Param limit query int false "Max items per page (1-100, default 50)"
// @Param cursor query string false "Pagination cursor from previous response"
// @Success 200 {object} paginatedResponse[connectionResponse]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Security BearerAuth
// @Router /v1/widget/connections [get]
func (h *ConnectAPIHandler) ListConnections(w http.ResponseWriter, r *http.Request) {
	sess, ok := middleware.ConnectSessionFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing session"})
		return
	}

	if !hasPermission(sess, "list") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return
	}

	org, _ := middleware.OrgFromContext(r.Context())

	if sess.IdentityID == nil {
		writeJSON(w, http.StatusOK, paginatedResponse[connectionResponse]{
			Data:    []connectionResponse{},
			HasMore: false,
		})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("org_id = ? AND identity_id = ? AND revoked_at IS NULL", org.ID, *sess.IdentityID)
	q = applyPagination(q, cursor, limit)

	var creds []model.Credential
	if err := q.Find(&creds).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list connections"})
		return
	}

	hasMore := len(creds) > limit
	if hasMore {
		creds = creds[:limit]
	}

	items := make([]connectionResponse, len(creds))
	for i, c := range creds {
		items[i] = connectionResponse{
			ID:         c.ID.String(),
			Label:      c.Label,
			ProviderID: c.ProviderID,
			BaseURL:    c.BaseURL,
			AuthScheme: c.AuthScheme,
			CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		}
		if p, ok := h.reg.GetProvider(c.ProviderID); ok {
			items[i].ProviderName = p.Name
		}
	}

	resp := paginatedResponse[connectionResponse]{
		Data:    items,
		HasMore: hasMore,
	}
	if hasMore {
		last := creds[len(creds)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		resp.NextCursor = &c
	}

	writeJSON(w, http.StatusOK, resp)
}

// CreateConnection handles POST /v1/widget/connections.
// @Summary Create a connection
// @Description Creates a new credential connection for the session's identity via the widget.
// @Tags widget
// @Accept json
// @Produce json
// @Param body body createConnectionRequest true "Connection details"
// @Success 201 {object} connectionResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/widget/connections [post]
func (h *ConnectAPIHandler) CreateConnection(w http.ResponseWriter, r *http.Request) {
	sess, ok := middleware.ConnectSessionFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing session"})
		return
	}

	if !hasPermission(sess, "create") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return
	}

	org, _ := middleware.OrgFromContext(r.Context())

	var req createConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.ProviderID == "" || req.APIKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider_id and api_key are required"})
		return
	}

	// Validate provider exists in registry
	provider, providerOK := h.reg.GetProvider(req.ProviderID)
	if !providerOK {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown provider: " + req.ProviderID})
		return
	}

	// Check against session's allowed_providers
	if len(sess.AllowedProviders) > 0 {
		allowed := false
		for _, p := range sess.AllowedProviders {
			if p == req.ProviderID {
				allowed = true
				break
			}
		}
		if !allowed {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "provider not allowed for this session"})
			return
		}
	}

	// Resolve base_url from provider
	baseURL := provider.API
	if baseURL == "" {
		var ok bool
		baseURL, ok = knownBaseURLs[req.ProviderID]
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no known base URL for provider: " + req.ProviderID})
			return
		}
	}

	// Resolve auth_scheme (default bearer)
	authScheme := "bearer"
	if scheme, ok := knownAuthSchemes[req.ProviderID]; ok {
		authScheme = scheme
	}

	// SSRF validate the base_url
	if err := proxy.ValidateBaseURL(baseURL); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid base_url: %v", err)})
		return
	}

	// Ensure identity exists
	if sess.IdentityID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session has no identity linked"})
		return
	}

	// Verify API key against provider
	verifyResult := h.reg.Verify(r.Context(), req.ProviderID, baseURL, authScheme, []byte(req.APIKey))
	if !verifyResult.Valid {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "api key verification failed: " + verifyResult.Error,
		})
		return
	}

	// Encrypt API key
	dek, err := crypto.GenerateDEK()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encryption failed"})
		return
	}

	encryptedKey, err := crypto.EncryptCredential([]byte(req.APIKey), dek)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encryption failed"})
		return
	}

	wrappedDEK, err := h.kms.Wrap(r.Context(), dek)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key wrapping failed"})
		return
	}

	// Zero plaintext DEK
	for i := range dek {
		dek[i] = 0
	}

	label := req.Label
	if label == "" {
		label = provider.Name
	}

	cred := model.Credential{
		ID:           uuid.New(),
		OrgID:        org.ID,
		Label:        label,
		BaseURL:      baseURL,
		AuthScheme:   authScheme,
		ProviderID:   req.ProviderID,
		IdentityID:   sess.IdentityID,
		EncryptedKey: encryptedKey,
		WrappedDEK:   wrappedDEK,
	}

	if err := h.db.Create(&cred).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store connection"})
		return
	}

	writeJSON(w, http.StatusCreated, connectionResponse{
		ID:           cred.ID.String(),
		Label:        cred.Label,
		ProviderID:   cred.ProviderID,
		ProviderName: provider.Name,
		BaseURL:      cred.BaseURL,
		AuthScheme:   cred.AuthScheme,
		CreatedAt:    cred.CreatedAt.Format(time.RFC3339),
	})
}

// DeleteConnection handles DELETE /v1/widget/connections/{id}.
// @Summary Delete a connection
// @Description Revokes (soft-deletes) a connection owned by the session's identity.
// @Tags widget
// @Produce json
// @Param id path string true "Connection ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/widget/connections/{id} [delete]
func (h *ConnectAPIHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	sess, ok := middleware.ConnectSessionFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing session"})
		return
	}

	if !hasPermission(sess, "delete") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return
	}

	org, _ := middleware.OrgFromContext(r.Context())
	connID := chi.URLParam(r, "id")

	if sess.IdentityID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session has no identity"})
		return
	}

	now := time.Now()
	result := h.db.Model(&model.Credential{}).
		Where("id = ? AND org_id = ? AND identity_id = ? AND revoked_at IS NULL",
			connID, org.ID, *sess.IdentityID).
		Update("revoked_at", &now)

	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete connection"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// VerifyConnection handles POST /v1/widget/connections/{id}/verify.
// @Summary Verify a connection
// @Description Decrypts the stored API key and verifies it against the provider.
// @Tags widget
// @Produce json
// @Param id path string true "Connection ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/widget/connections/{id}/verify [post]
func (h *ConnectAPIHandler) VerifyConnection(w http.ResponseWriter, r *http.Request) {
	sess, ok := middleware.ConnectSessionFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing session"})
		return
	}

	if !hasPermission(sess, "verify") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return
	}

	org, _ := middleware.OrgFromContext(r.Context())
	connID := chi.URLParam(r, "id")

	if sess.IdentityID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session has no identity"})
		return
	}

	var cred model.Credential
	if err := h.db.Where("id = ? AND org_id = ? AND identity_id = ? AND revoked_at IS NULL",
		connID, org.ID, *sess.IdentityID).First(&cred).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	// Decrypt the API key
	dek, err := h.kms.Unwrap(r.Context(), cred.WrappedDEK)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "decryption failed"})
		return
	}

	apiKey, err := crypto.DecryptCredential(cred.EncryptedKey, dek)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "decryption failed"})
		return
	}

	// Zero DEK
	for i := range dek {
		dek[i] = 0
	}

	// Verify the key
	result := h.reg.Verify(r.Context(), cred.ProviderID, cred.BaseURL, cred.AuthScheme, apiKey)

	// Zero API key
	for i := range apiKey {
		apiKey[i] = 0
	}

	writeJSON(w, http.StatusOK, result)
}

type widgetResourceResponse struct {
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Icon        string `json:"icon,omitempty"`
}

type widgetIntegrationResponse struct {
	ID                string                   `json:"id"`
	UniqueKey         string                   `json:"unique_key"`
	Provider          string                   `json:"provider"`
	DisplayName       string                   `json:"display_name"`
	AuthMode          string                   `json:"auth_mode"`
	ConnectionID      *string                  `json:"connection_id"`
	NangoConnectionID *string                  `json:"nango_connection_id,omitempty"`
	Resources         []widgetResourceResponse `json:"resources"`
	SelectedResources map[string][]string      `json:"selected_resources,omitempty"`
}

// ListIntegrations handles GET /v1/widget/integrations.
// @Summary List org integrations for widget
// @Description Returns integrations configured by the org for the current session.
// @Tags widget
// @Produce json
// @Success 200 {array} widgetIntegrationResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/widget/integrations [get]
func (h *ConnectAPIHandler) ListIntegrations(w http.ResponseWriter, r *http.Request) {
	sess, ok := middleware.ConnectSessionFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing session"})
		return
	}

	org, _ := middleware.OrgFromContext(r.Context())

	var integrations []model.Integration
	if err := h.db.Where("org_id = ? AND deleted_at IS NULL", org.ID).Find(&integrations).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list integrations"})
		return
	}

	// Build a map of integration_id → connection for the current identity
	connectionMap := make(map[uuid.UUID]model.Connection)
	if sess.IdentityID != nil {
		var conns []model.Connection
		if err := h.db.Where("org_id = ? AND identity_id = ? AND revoked_at IS NULL", org.ID, *sess.IdentityID).Find(&conns).Error; err == nil {
			for _, c := range conns {
				connectionMap[c.IntegrationID] = c
			}
		}
	}

	resp := make([]widgetIntegrationResponse, 0, len(integrations))
	for _, integ := range integrations {
		authMode := ""
		if p, found := h.nango.GetProvider(integ.Provider); found {
			authMode = p.AuthMode
		}
		item := widgetIntegrationResponse{
			ID:          integ.ID.String(),
			UniqueKey:   integ.UniqueKey,
			Provider:    integ.Provider,
			DisplayName: integ.DisplayName,
			AuthMode:    authMode,
			Resources:   []widgetResourceResponse{},
		}
		if conn, ok := connectionMap[integ.ID]; ok {
			connID := conn.ID.String()
			item.ConnectionID = &connID
			item.NangoConnectionID = &conn.NangoConnectionID

			// Extract selected resources from connection meta
			if conn.Meta != nil {
				if raw, exists := conn.Meta["resources"]; exists {
					if resMap, ok := raw.(map[string]interface{}); ok {
						selected := make(map[string][]string, len(resMap))
						for k, v := range resMap {
							if arr, ok := v.([]interface{}); ok {
								ids := make([]string, 0, len(arr))
								for _, id := range arr {
									if s, ok := id.(string); ok {
										ids = append(ids, s)
									}
								}
								selected[k] = ids
							}
						}
						if len(selected) > 0 {
							item.SelectedResources = selected
						}
					}
				}
			}
		}

		// Add resource configurations from catalog
		if resourceTypes := h.catalog.ListResourceTypes(integ.Provider); resourceTypes != nil {
			for typeKey, resDef := range resourceTypes {
				item.Resources = append(item.Resources, widgetResourceResponse{
					Type:        typeKey,
					DisplayName: resDef.DisplayName,
					Description: resDef.Description,
					Icon:        resDef.Icon,
				})
			}
		}

		resp = append(resp, item)
	}

	writeJSON(w, http.StatusOK, resp)
}

type connectSessionTokenResponse struct {
	Token             string `json:"token"`
	ProviderConfigKey string `json:"provider_config_key"`
}

type createIntegrationConnectionRequest struct {
	NangoConnectionID string                 `json:"nango_connection_id"`
	Resources         map[string][]string    `json:"resources,omitempty"`
}

// CreateIntegrationConnectSession handles POST /v1/widget/integrations/{id}/connect-session.
// @Summary Create a connect session for an integration
// @Description Creates a Nango connect session scoped to a specific integration.
// @Tags widget
// @Produce json
// @Param id path string true "Integration ID"
// @Success 200 {object} connectSessionTokenResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/widget/integrations/{id}/connect-session [post]
func (h *ConnectAPIHandler) CreateIntegrationConnectSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := middleware.ConnectSessionFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing session"})
		return
	}

	if !hasPermission(sess, "create") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return
	}

	org, _ := middleware.OrgFromContext(r.Context())

	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	integUUID, err := uuid.Parse(integID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid integration id"})
		return
	}

	var integ model.Integration
	if err := h.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", integUUID, org.ID).First(&integ).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
		return
	}

	// Build the org-namespaced Nango unique key
	nangoUniqueKey := fmt.Sprintf("%s_%s", org.ID.String(), integ.UniqueKey)

	// Use session external_id or identity_id as end_user for Nango
	endUserID := sess.ExternalID
	if endUserID == "" && sess.IdentityID != nil {
		endUserID = sess.IdentityID.String()
	}
	if endUserID == "" {
		endUserID = sess.ID.String()
	}

	sessionResp, err := h.nango.CreateConnectSession(r.Context(), nango.CreateConnectSessionRequest{
		EndUser:             nango.ConnectSessionEndUser{ID: endUserID},
		AllowedIntegrations: []string{nangoUniqueKey},
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create connect session: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, connectSessionTokenResponse{
		Token:             sessionResp.Token,
		ProviderConfigKey: nangoUniqueKey,
	})
}

// CreateIntegrationConnection handles POST /v1/widget/integrations/{id}/connections.
// @Summary Create a connection via the widget
// @Description Stores a new connection record after a successful Nango auth flow.
// @Tags widget
// @Accept json
// @Produce json
// @Param id path string true "Integration ID"
// @Param body body createIntegrationConnectionRequest true "Connection details"
// @Success 201 {object} integConnResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/widget/integrations/{id}/connections [post]
func (h *ConnectAPIHandler) CreateIntegrationConnection(w http.ResponseWriter, r *http.Request) {
	logger := slog.With("handler", "CreateIntegrationConnection")

	sess, ok := middleware.ConnectSessionFromContext(r.Context())
	if !ok {
		logger.Warn("request missing session")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing session"})
		return
	}

	if !hasPermission(sess, "create") {
		logger.Warn("permission denied",
			"identity_id", sess.IdentityID,
			"session_id", sess.ID,
		)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return
	}

	org, _ := middleware.OrgFromContext(r.Context())

	integID := chi.URLParam(r, "id")
	if integID == "" {
		logger.Warn("missing integration id")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	integUUID, err := uuid.Parse(integID)
	if err != nil {
		logger.Warn("invalid integration id format", "error", err.Error())
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid integration id"})
		return
	}

	logger = logger.With(
		"integration_id", integID,
		"org_id", org.ID,
	)
	logger.Info("looking up integration")

	var integ model.Integration
	if err := h.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", integUUID, org.ID).First(&integ).Error; err != nil {
		logger.Error("integration not found", "error", err.Error())
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
		return
	}

	logger = logger.With(
		"provider", integ.Provider,
		"integration_unique_key", integ.UniqueKey,
	)
	logger.Info("integration found")

	var req createIntegrationConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("invalid request body", "error", err.Error())
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.NangoConnectionID == "" {
		logger.Warn("missing nango_connection_id")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nango_connection_id is required"})
		return
	}

	logger = logger.With(
		"nango_connection_id", req.NangoConnectionID,
		"identity_id", sess.IdentityID,
	)

	// Enforce one active connection per identity per integration
	if sess.IdentityID != nil {
		var existing model.Connection
		err := h.db.Where("org_id = ? AND integration_id = ? AND identity_id = ? AND revoked_at IS NULL",
			org.ID, integ.ID, *sess.IdentityID).First(&existing).Error
		if err == nil {
			logger.Warn("duplicate connection attempt",
				"existing_connection_id", existing.ID,
			)
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already connected to this integration"})
			return
		}
	}

	// Verify connection exists in Nango before storing reference
	nangoProviderConfigKey := fmt.Sprintf("%s_%s", org.ID.String(), integ.UniqueKey)
	if _, err := h.nango.GetConnection(r.Context(), req.NangoConnectionID, nangoProviderConfigKey); err != nil {
		logger.Warn("nango_connection_id not found in Nango",
			"error", err.Error(),
			"nango_provider_config_key", nangoProviderConfigKey,
		)
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "nango_connection_id not found — complete the auth flow before storing the connection",
		})
		return
	}

	meta := model.JSON{}
	if len(req.Resources) > 0 {
		meta["resources"] = req.Resources
		logger.Info("creating connection with pre-selected resources",
			"resource_types", len(req.Resources),
		)
	} else {
		logger.Info("creating connection without resources (resource selection pending or skipped)")
	}

	conn := model.Connection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		IntegrationID:     integ.ID,
		NangoConnectionID: req.NangoConnectionID,
		IdentityID:        sess.IdentityID,
		Meta:              meta,
	}

	if err := h.db.Create(&conn).Error; err != nil {
		logger.Error("failed to create connection in database", "error", err.Error())
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
		return
	}

	logger.Info("connection created successfully",
		"connection_id", conn.ID,
		"has_resources", len(req.Resources) > 0,
	)

	writeJSON(w, http.StatusCreated, toIntegConnResponse(conn))
}

// PatchIntegrationConnection handles PATCH /v1/widget/integrations/{id}/connections/{connectionId}.
// @Summary Update connection resources
// @Description Updates the resources configured for a connection (resource selection).
// @Tags widget
// @Accept json
// @Produce json
// @Param id path string true "Integration ID"
// @Param connectionId path string true "Connection ID"
// @Param body body patchIntegrationConnectionRequest true "Update parameters"
// @Success 200 {object} integConnResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/widget/integrations/{id}/connections/{connectionId} [patch]
func (h *ConnectAPIHandler) PatchIntegrationConnection(w http.ResponseWriter, r *http.Request) {
	logger := slog.With("handler", "PatchIntegrationConnection")

	sess, ok := middleware.ConnectSessionFromContext(r.Context())
	if !ok {
		logger.Warn("request missing session")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing session"})
		return
	}

	if !hasPermission(sess, "update") {
		logger.Warn("permission denied",
			"identity_id", sess.IdentityID,
			"session_id", sess.ID,
		)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return
	}

	org, _ := middleware.OrgFromContext(r.Context())
	integID := chi.URLParam(r, "id")
	connID := chi.URLParam(r, "connectionId")

	logger = logger.With(
		"integration_id", integID,
		"connection_id", connID,
		"org_id", org.ID,
		"identity_id", sess.IdentityID,
	)
	logger.Info("handling patch connection resources request")

	if sess.IdentityID == nil {
		logger.Warn("session has no identity")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session has no identity"})
		return
	}

	// Parse integration ID
	integUUID, err := uuid.Parse(integID)
	if err != nil {
		logger.Warn("invalid integration id format", "error", err.Error())
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid integration id"})
		return
	}

	// Verify integration exists and belongs to org
	var integ model.Integration
	if err := h.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", integUUID, org.ID).First(&integ).Error; err != nil {
		logger.Error("integration not found", "error", err.Error())
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
		return
	}

	logger = logger.With(
		"provider", integ.Provider,
		"integration_unique_key", integ.UniqueKey,
	)

	var req patchIntegrationConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("invalid request body", "error", err.Error())
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Log resource update details
	if len(req.Resources) > 0 {
		totalResources := 0
		for _, items := range req.Resources {
			totalResources += len(items)
		}
		logger.Info("updating connection resources",
			"resource_types", len(req.Resources),
			"total_resource_count", totalResources,
			"resource_types_list", getResourceTypes(req.Resources),
		)
	} else {
		logger.Info("clearing connection resources (full access granted)")
	}

	// Look up the connection
	var conn model.Connection
	if err := h.db.Where("id = ? AND org_id = ? AND integration_id = ? AND identity_id = ? AND revoked_at IS NULL",
		connID, org.ID, integ.ID, *sess.IdentityID).First(&conn).Error; err != nil {
		logger.Error("connection not found",
			"error", err.Error(),
		)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	logger = logger.With(
		"nango_connection_id", conn.NangoConnectionID,
	)

	// Update meta with resources
	meta := conn.Meta
	if meta == nil {
		meta = model.JSON{}
	}

	if len(req.Resources) > 0 {
		meta["resources"] = req.Resources
	} else {
		delete(meta, "resources")
	}

	conn.Meta = meta
	conn.UpdatedAt = time.Now()

	if err := h.db.Save(&conn).Error; err != nil {
		logger.Error("failed to update connection in database", "error", err.Error())
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update connection"})
		return
	}

	logger.Info("connection resources updated successfully")

	writeJSON(w, http.StatusOK, toIntegConnResponse(conn))
}

// getResourceTypes returns a slice of resource type keys from the resources map.
func getResourceTypes(resources map[string][]string) []string {
	types := make([]string, 0, len(resources))
	for t := range resources {
		types = append(types, t)
	}
	return types
}

type patchIntegrationConnectionRequest struct {
	Resources map[string][]string `json:"resources,omitempty"`
}

// DeleteIntegrationConnection handles DELETE /v1/widget/integrations/{id}/connections/{connectionId}.
// @Summary Delete an integration connection
// @Description Revokes (soft-deletes) an integration connection owned by the session's identity.
// @Tags widget
// @Produce json
// @Param id path string true "Integration ID"
// @Param connectionId path string true "Connection ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/widget/integrations/{id}/connections/{connectionId} [delete]
func (h *ConnectAPIHandler) DeleteIntegrationConnection(w http.ResponseWriter, r *http.Request) {
	sess, ok := middleware.ConnectSessionFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing session"})
		return
	}

	if !hasPermission(sess, "delete") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return
	}

	org, _ := middleware.OrgFromContext(r.Context())
	connID := chi.URLParam(r, "connectionId")

	if sess.IdentityID == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session has no identity"})
		return
	}

	// Get integration ID from URL and verify it matches the connection
	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	integUUID, err := uuid.Parse(integID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid integration id"})
		return
	}

	// Look up the connection (with its integration) to get Nango IDs
	var conn model.Connection
	if err := h.db.Preload("Integration").Where("id = ? AND org_id = ? AND integration_id = ? AND identity_id = ? AND revoked_at IS NULL",
		connID, org.ID, integUUID, *sess.IdentityID).First(&conn).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	// Build the org-namespaced Nango provider config key
	nangoProviderConfigKey := fmt.Sprintf("%s_%s", org.ID.String(), conn.Integration.UniqueKey)

	// Delete from Nango
	if err := h.nango.DeleteConnection(r.Context(), conn.NangoConnectionID, nangoProviderConfigKey); err != nil {
		slog.Error("nango: delete connection failed, proceeding with local revocation",
			"error", err, "connection_id", connID, "nango_connection_id", conn.NangoConnectionID)
	}

	// Soft-delete locally
	now := time.Now()
	result := h.db.Model(&model.Connection{}).
		Where("id = ? AND revoked_at IS NULL", connID).
		Update("revoked_at", &now)
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete connection"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "connection not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ListAvailableResources handles GET /v1/widget/integrations/{id}/resources/{type}/available.
// @Summary List available resources
// @Description Fetches available resources of a specific type from the provider API.
// @Tags widget
// @Produce json
// @Param id path string true "Integration ID"
// @Param type path string true "Resource type (e.g., channel, repo, folder)"
// @Param nango_connection_id query string true "Nango connection ID from OAuth flow"
// @Success 200 {object} resources.DiscoveryResult
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/widget/integrations/{id}/resources/{type}/available [get]
func (h *ConnectAPIHandler) ListAvailableResources(w http.ResponseWriter, r *http.Request) {
	logger := slog.With("handler", "ListAvailableResources")

	sess, ok := middleware.ConnectSessionFromContext(r.Context())
	if !ok {
		logger.Warn("request missing session")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing session"})
		return
	}

	if !hasPermission(sess, "list") {
		logger.Warn("permission denied",
			"identity_id", sess.IdentityID,
			"session_id", sess.ID,
		)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		return
	}

	org, _ := middleware.OrgFromContext(r.Context())

	integID := chi.URLParam(r, "id")
	resourceType := chi.URLParam(r, "type")
	nangoConnID := r.URL.Query().Get("nango_connection_id")

	logger = logger.With(
		"integration_id", integID,
		"resource_type", resourceType,
		"nango_connection_id", nangoConnID,
		"org_id", org.ID,
	)
	logger.Info("handling list available resources request")

	if integID == "" {
		logger.Warn("missing integration id")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	if resourceType == "" {
		logger.Warn("missing resource type")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "resource type required"})
		return
	}

	integUUID, err := uuid.Parse(integID)
	if err != nil {
		logger.Warn("invalid integration id format",
			"error", err.Error(),
		)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid integration id"})
		return
	}

	// Get integration to find provider
	var integ model.Integration
	if err := h.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", integUUID, org.ID).First(&integ).Error; err != nil {
		logger.Error("integration not found",
			"error", err.Error(),
		)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
		return
	}

	logger = logger.With(
		"provider", integ.Provider,
		"integration_unique_key", integ.UniqueKey,
	)
	logger.Info("integration found")

	// Check if provider supports resource discovery
	if !h.discovery.HasDiscovery(integ.Provider) {
		logger.Warn("resource discovery not supported for provider")
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("resource discovery not supported for provider %s", integ.Provider),
		})
		return
	}

	// Get nango_connection_id from query param
	if nangoConnID == "" {
		logger.Warn("missing nango_connection_id query parameter")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nango_connection_id query parameter required"})
		return
	}

	// Build provider config key
	nangoProviderConfigKey := fmt.Sprintf("%s_%s", org.ID.String(), integ.UniqueKey)
	logger = logger.With("nango_provider_config_key", nangoProviderConfigKey)

	// Discover resources
	logger.Info("starting resource discovery")
	result, err := h.discovery.Discover(r.Context(), integ.Provider, resourceType, nangoProviderConfigKey, nangoConnID)
	if err != nil {
		logger.Error("resource discovery failed",
			"error", err.Error(),
		)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	logger.Info("resource discovery successful",
		"resource_count", len(result.Resources),
	)

	writeJSON(w, http.StatusOK, result)
}

// hasPermission checks if the session has a specific permission.
// If no permissions are set on the session, all operations are allowed.
func hasPermission(sess *model.ConnectSession, perm string) bool {
	if len(sess.Permissions) == 0 {
		return true
	}
	for _, p := range sess.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}
