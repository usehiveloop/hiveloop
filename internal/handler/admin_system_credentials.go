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

	"github.com/usehiveloop/hiveloop/internal/cache"
	"github.com/usehiveloop/hiveloop/internal/credentials"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/proxy"
	"github.com/usehiveloop/hiveloop/internal/registry"
)

// AdminSystemCredentialsHandler exposes the admin-only CRUD surface for
// system credentials — the platform-owned keys used by agents that opted out
// of BYOK. These credentials are hidden from every org-scoped endpoint; the
// only way to create, list, or revoke them is via /admin/v1/system-credentials.
type AdminSystemCredentialsHandler struct {
	db           *gorm.DB
	kms          *crypto.KeyWrapper
	cacheManager *cache.Manager
}

// NewAdminSystemCredentialsHandler constructs the handler. It requires the
// KMS wrapper because create has to encrypt the API key, mirroring the
// user-facing CredentialHandler.
func NewAdminSystemCredentialsHandler(db *gorm.DB, kms *crypto.KeyWrapper, cm *cache.Manager) *AdminSystemCredentialsHandler {
	return &AdminSystemCredentialsHandler{db: db, kms: kms, cacheManager: cm}
}

type createSystemCredentialRequest struct {
	Label      string `json:"label"`
	ProviderID string `json:"provider_id"`
	BaseURL    string `json:"base_url"`
	AuthScheme string `json:"auth_scheme"`
	APIKey     string `json:"api_key"`
}

type systemCredentialResponse struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	ProviderID string  `json:"provider_id"`
	BaseURL    string  `json:"base_url"`
	AuthScheme string  `json:"auth_scheme"`
	CreatedAt  string  `json:"created_at"`
	RevokedAt  *string `json:"revoked_at,omitempty"`
}

// Create handles POST /admin/v1/system-credentials.
// @Summary Create a system credential
// @Description Creates a platform-owned credential used by agents that opted out of BYOK. Admin-only.
// @Tags admin
// @Accept json
// @Produce json
// @Param body body createSystemCredentialRequest true "Credential details"
// @Success 201 {object} systemCredentialResponse
// @Failure 400 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/system-credentials [post]
func (h *AdminSystemCredentialsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createSystemCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.APIKey == "" || req.ProviderID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider_id and api_key are required"})
		return
	}

	provider, ok := registry.Global().GetProvider(req.ProviderID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown provider_id %q", req.ProviderID)})
		return
	}

	if req.BaseURL == "" {
		if provider.API == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %q has no default base URL; please provide base_url", req.ProviderID)})
			return
		}
		req.BaseURL = provider.API
	}
	if req.AuthScheme == "" {
		if scheme, ok := providerAuthSchemes[req.ProviderID]; ok {
			req.AuthScheme = scheme
		} else {
			req.AuthScheme = "bearer"
		}
	}

	validSchemes := map[string]bool{"bearer": true, "x-api-key": true, "api-key": true, "query_param": true}
	if !validSchemes[req.AuthScheme] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid auth_scheme"})
		return
	}
	if err := proxy.ValidateBaseURL(req.BaseURL); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid base_url: %v", err)})
		return
	}

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
	// Scrub the DEK from memory as soon as it's wrapped.
	for i := range dek {
		dek[i] = 0
	}

	cred := model.Credential{
		ID:           uuid.New(),
		OrgID:        credentials.PlatformOrgID,
		IsSystem:     true,
		Label:        req.Label,
		ProviderID:   req.ProviderID,
		BaseURL:      req.BaseURL,
		AuthScheme:   req.AuthScheme,
		EncryptedKey: encryptedKey,
		WrappedDEK:   wrappedDEK,
	}
	if err := h.db.WithContext(r.Context()).Create(&cred).Error; err != nil {
		slog.Error("admin: failed to create system credential", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store credential"})
		return
	}

	slog.Info("admin: system credential created", "credential_id", cred.ID, "provider_id", cred.ProviderID, "label", cred.Label)
	writeJSON(w, http.StatusCreated, toSystemCredentialResponse(cred))
}

// List handles GET /admin/v1/system-credentials.
// @Summary List system credentials
// @Description Returns every platform-owned credential. Admin-only.
// @Tags admin
// @Produce json
// @Success 200 {array} systemCredentialResponse
// @Security BearerAuth
// @Router /admin/v1/system-credentials [get]
func (h *AdminSystemCredentialsHandler) List(w http.ResponseWriter, r *http.Request) {
	var creds []model.Credential
	if err := h.db.WithContext(r.Context()).
		Where("is_system = ?", true).
		Order("created_at DESC").
		Find(&creds).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list system credentials"})
		return
	}
	out := make([]systemCredentialResponse, len(creds))
	for i, c := range creds {
		out[i] = toSystemCredentialResponse(c)
	}
	writeJSON(w, http.StatusOK, out)
}

// Revoke handles POST /admin/v1/system-credentials/{id}/revoke.
// @Summary Revoke a system credential
// @Description Revokes a platform-owned credential. Agents that picked this credential will fail their next resolution and fall back to another system credential (or return a 503 if none remain).
// @Tags admin
// @Produce json
// @Param id path string true "Credential ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/system-credentials/{id}/revoke [post]
func (h *AdminSystemCredentialsHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now()

	result := h.db.WithContext(r.Context()).
		Model(&model.Credential{}).
		Where("id = ? AND is_system = ? AND revoked_at IS NULL", id, true).
		Update("revoked_at", now)
	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke credential"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "system credential not found or already revoked"})
		return
	}

	if h.cacheManager != nil {
		_ = h.cacheManager.InvalidateCredential(r.Context(), id)
	}

	slog.Info("admin: system credential revoked", "credential_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func toSystemCredentialResponse(c model.Credential) systemCredentialResponse {
	resp := systemCredentialResponse{
		ID:         c.ID.String(),
		Label:      c.Label,
		ProviderID: c.ProviderID,
		BaseURL:    c.BaseURL,
		AuthScheme: c.AuthScheme,
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
	}
	if c.RevokedAt != nil {
		s := c.RevokedAt.Format(time.RFC3339)
		resp.RevokedAt = &s
	}
	return resp
}
