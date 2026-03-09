package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/useportal/llmvault/internal/crypto"
	"github.com/useportal/llmvault/internal/middleware"
	"github.com/useportal/llmvault/internal/model"
	"github.com/useportal/llmvault/internal/proxy"
	"github.com/useportal/llmvault/internal/registry"
)

// ConnectAPIHandler serves the Connect widget's API endpoints.
type ConnectAPIHandler struct {
	db  *gorm.DB
	kms *crypto.KeyWrapper
	reg *registry.Registry
}

// NewConnectAPIHandler creates a new connect API handler.
func NewConnectAPIHandler(db *gorm.DB, kms *crypto.KeyWrapper, reg *registry.Registry) *ConnectAPIHandler {
	return &ConnectAPIHandler{db: db, kms: kms, reg: reg}
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

// SessionInfo handles GET /connect/api/session.
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

// ListProviders handles GET /connect/api/providers.
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

// ListConnections handles GET /connect/api/connections.
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
		writeJSON(w, http.StatusOK, []connectionResponse{})
		return
	}

	var creds []model.Credential
	h.db.Where("org_id = ? AND identity_id = ? AND revoked_at IS NULL", org.ID, *sess.IdentityID).Find(&creds)

	resp := make([]connectionResponse, len(creds))
	for i, c := range creds {
		resp[i] = connectionResponse{
			ID:         c.ID.String(),
			Label:      c.Label,
			ProviderID: c.ProviderID,
			BaseURL:    c.BaseURL,
			AuthScheme: c.AuthScheme,
			CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		}
		if p, ok := h.reg.GetProvider(c.ProviderID); ok {
			resp[i].ProviderName = p.Name
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// CreateConnection handles POST /connect/api/connections.
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

// DeleteConnection handles DELETE /connect/api/connections/{id}.
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

// VerifyConnection handles POST /connect/api/connections/{id}/verify.
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
	result := registry.Verify(r.Context(), cred.ProviderID, cred.BaseURL, cred.AuthScheme, apiKey)

	// Zero API key
	for i := range apiKey {
		apiKey[i] = 0
	}

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
