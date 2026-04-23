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
	"github.com/usehiveloop/hiveloop/internal/counter"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/proxy"
	"github.com/usehiveloop/hiveloop/internal/registry"
)

type CredentialHandler struct {
	db           *gorm.DB
	kms          *crypto.KeyWrapper
	cacheManager *cache.Manager
	counter      *counter.Counter
}

func NewCredentialHandler(db *gorm.DB, kms *crypto.KeyWrapper, cm *cache.Manager, ctr *counter.Counter) *CredentialHandler {
	return &CredentialHandler{db: db, kms: kms, cacheManager: cm, counter: ctr}
}

// providerAuthSchemes maps provider IDs to their default auth schemes.
// Providers not listed default to "bearer".
var providerAuthSchemes = map[string]string{
	"anthropic":      "x-api-key",
	"google":         "query_param",
	"azure":          "api-key",
	"amazon-bedrock": "bearer",
}

type createCredentialRequest struct {
	Label          string     `json:"label"`
	ProviderID     string     `json:"provider_id"`
	BaseURL        string     `json:"base_url"`
	AuthScheme     string     `json:"auth_scheme"`
	APIKey         string     `json:"api_key"`
	ExternalID     *string    `json:"external_id,omitempty"`
	Remaining      *int64     `json:"remaining,omitempty"`
	RefillAmount   *int64     `json:"refill_amount,omitempty"`
	RefillInterval *string    `json:"refill_interval,omitempty"`
	Meta           model.JSON `json:"meta,omitempty"`
}

type credentialResponse struct {
	ID             string     `json:"id"`
	Label          string     `json:"label"`
	BaseURL        string     `json:"base_url"`
	AuthScheme     string     `json:"auth_scheme"`
	ProviderID     string     `json:"provider_id,omitempty"`
	Remaining      *int64     `json:"remaining,omitempty"`
	RefillAmount   *int64     `json:"refill_amount,omitempty"`
	RefillInterval *string    `json:"refill_interval,omitempty"`
	Meta           model.JSON `json:"meta,omitempty"`
	RequestCount   int64      `json:"request_count"`
	LastUsedAt     *string    `json:"last_used_at,omitempty"`
	CreatedAt      string     `json:"created_at"`
	RevokedAt      *string    `json:"revoked_at,omitempty"`
}

// Create handles POST /v1/credentials.
// @Summary Create a credential
// @Description Stores an encrypted LLM API credential for the current organization.
// @Tags credentials
// @Accept json
// @Produce json
// @Param body body createCredentialRequest true "Credential details"
// @Success 201 {object} credentialResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/credentials [post]
func (h *CredentialHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req createCredentialRequest
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
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider %q does not have a base URL configured; please provide base_url", req.ProviderID)})
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

	for i := range dek {
		dek[i] = 0
	}

	if req.RefillInterval != nil {
		if _, err := time.ParseDuration(*req.RefillInterval); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid refill_interval: must be a valid Go duration (e.g. 1h, 24h)"})
			return
		}
	}

	cred := model.Credential{
		ID:             uuid.New(),
		OrgID:          org.ID,
		Label:          req.Label,
		BaseURL:        req.BaseURL,
		AuthScheme:     req.AuthScheme,
		ProviderID:     req.ProviderID,
		EncryptedKey:   encryptedKey,
		WrappedDEK:     wrappedDEK,
		Remaining:      req.Remaining,
		RefillAmount:   req.RefillAmount,
		RefillInterval: req.RefillInterval,
		Meta:           req.Meta,
	}

	if err := h.db.Create(&cred).Error; err != nil {
		slog.Error("failed to store credential", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store credential"})
		return
	}

	slog.Info("credential created", "org_id", org.ID, "credential_id", cred.ID, "provider_id", req.ProviderID, "label", req.Label)

	if cred.Remaining != nil && h.counter != nil {
		_ = h.counter.SeedCredential(r.Context(), cred.ID.String(), *cred.Remaining)
	}

	credResp := credentialResponse{
		ID:             cred.ID.String(),
		Label:          cred.Label,
		BaseURL:        cred.BaseURL,
		AuthScheme:     cred.AuthScheme,
		ProviderID:     cred.ProviderID,
		Remaining:      cred.Remaining,
		RefillAmount:   cred.RefillAmount,
		RefillInterval: cred.RefillInterval,
		Meta:           cred.Meta,
		CreatedAt:      cred.CreatedAt.Format(time.RFC3339),
	}
	writeJSON(w, http.StatusCreated, credResp)
}
