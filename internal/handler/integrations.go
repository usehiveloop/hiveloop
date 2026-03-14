package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/useportal/llmvault/internal/middleware"
	"github.com/useportal/llmvault/internal/model"
	"github.com/useportal/llmvault/internal/nango"
)

// IntegrationHandler manages integration CRUD operations.
type IntegrationHandler struct {
	db    *gorm.DB
	nango *nango.Client
}

// NewIntegrationHandler creates a new integration handler.
func NewIntegrationHandler(db *gorm.DB, nangoClient *nango.Client) *IntegrationHandler {
	return &IntegrationHandler{db: db, nango: nangoClient}
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
	Provider    string     `json:"provider"`
	DisplayName string     `json:"display_name"`
	Meta        model.JSON `json:"meta,omitempty"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

func toIntegrationResponse(integ model.Integration) integrationResponse {
	return integrationResponse{
		ID:          integ.ID.String(),
		Provider:    integ.Provider,
		DisplayName: integ.DisplayName,
		Meta:        integ.Meta,
		CreatedAt:   integ.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   integ.UpdatedAt.Format(time.RFC3339),
	}
}

// nangoKey returns the org-namespaced provider config key for Nango.
func nangoKey(orgID uuid.UUID, uniqueKey string) string {
	return fmt.Sprintf("%s_%s", orgID.String(), uniqueKey)
}

// validateCredentials validates the credentials object against the provider's auth_mode.
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
		// If provider has static client registration, client_id and client_secret are required
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
		// These auth modes do not require credentials — credentials must be absent/nil
		if creds != nil {
			return fmt.Errorf("credentials must not be provided for %s auth mode", mode)
		}

	default:
		// Unknown auth mode — allow without credentials for forward compatibility
		if creds != nil {
			return fmt.Errorf("credentials must not be provided for unknown auth mode %q", mode)
		}
	}

	return nil
}

// Create handles POST /v1/integrations.
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
	if h.nango != nil {
		provider, found := h.nango.GetProvider(req.Provider)
		if !found {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider %q", req.Provider)})
			return
		}

		// Validate credentials against provider's auth_mode
		if err := validateCredentials(provider, req.Credentials); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	// Auto-generate unique_key (internal only, never exposed to users)
	integID := uuid.New()
	uniqueKey := fmt.Sprintf("%s-%s", req.Provider, integID.String()[:8])

	// Push to Nango first (source of truth for OAuth credentials)
	if h.nango != nil {
		nangoReq := nango.CreateIntegrationRequest{
			UniqueKey:   nangoKey(org.ID, uniqueKey),
			Provider:    req.Provider,
			Credentials: req.Credentials,
		}
		if err := h.nango.CreateIntegration(r.Context(), nangoReq); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to create integration in Nango: " + err.Error()})
			return
		}
	}

	integ := model.Integration{
		ID:          integID,
		OrgID:       org.ID,
		UniqueKey:   uniqueKey,
		Provider:    req.Provider,
		DisplayName: req.DisplayName,
		Meta:        req.Meta,
	}

	if err := h.db.Create(&integ).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create integration"})
		return
	}

	writeJSON(w, http.StatusCreated, toIntegrationResponse(integ))
}

// Get handles GET /v1/integrations/{id}.
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

	writeJSON(w, http.StatusOK, toIntegrationResponse(integ))
}

// List handles GET /v1/integrations.
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

	// If credentials provided, validate and push to Nango
	if req.Credentials != nil && h.nango != nil {
		provider, found := h.nango.GetProvider(integ.Provider)
		if !found {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider %q", integ.Provider)})
			return
		}
		if err := validateCredentials(provider, req.Credentials); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		nangoReq := nango.UpdateIntegrationRequest{
			Credentials: req.Credentials,
		}
		if err := h.nango.UpdateIntegration(r.Context(), nangoKey(org.ID, integ.UniqueKey), nangoReq); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to update integration in Nango: " + err.Error()})
			return
		}
	}

	updates := map[string]any{}
	if req.DisplayName != nil {
		updates["display_name"] = *req.DisplayName
	}
	if req.Meta != nil {
		updates["meta"] = req.Meta
	}
	if len(updates) > 0 {
		if err := h.db.Model(&integ).Updates(updates).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update integration"})
			return
		}
	}

	// Reload
	h.db.Where("id = ?", integ.ID).First(&integ)

	writeJSON(w, http.StatusOK, toIntegrationResponse(integ))
}

// Delete handles DELETE /v1/integrations/{id}.
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

	// Remove from Nango
	if h.nango != nil {
		if err := h.nango.DeleteIntegration(r.Context(), nangoKey(org.ID, integ.UniqueKey)); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to delete integration from Nango: " + err.Error()})
			return
		}
	}

	// Soft-delete
	now := time.Now()
	if err := h.db.Model(&integ).Update("deleted_at", now).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete integration"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
