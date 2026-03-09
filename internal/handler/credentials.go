package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/useportal/llmvault/internal/cache"
	"github.com/useportal/llmvault/internal/counter"
	"github.com/useportal/llmvault/internal/crypto"
	"github.com/useportal/llmvault/internal/middleware"
	"github.com/useportal/llmvault/internal/model"
	"github.com/useportal/llmvault/internal/proxy"
	"github.com/useportal/llmvault/internal/registry"
)

// CredentialHandler manages credential CRUD operations.
type CredentialHandler struct {
	db           *gorm.DB
	kms          *crypto.KeyWrapper
	cacheManager *cache.Manager
	counter      *counter.Counter
}

// NewCredentialHandler creates a new credential handler.
func NewCredentialHandler(db *gorm.DB, kms *crypto.KeyWrapper, cm *cache.Manager, ctr *counter.Counter) *CredentialHandler {
	return &CredentialHandler{db: db, kms: kms, cacheManager: cm, counter: ctr}
}

type createCredentialRequest struct {
	Label          string     `json:"label"`
	BaseURL        string     `json:"base_url"`
	AuthScheme     string     `json:"auth_scheme"`
	APIKey         string     `json:"api_key"`
	IdentityID     *string    `json:"identity_id,omitempty"`
	ExternalID     *string    `json:"external_id,omitempty"` // auto-upserts identity
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
	IdentityID     *string    `json:"identity_id,omitempty"`
	Remaining      *int64     `json:"remaining,omitempty"`
	RefillAmount   *int64     `json:"refill_amount,omitempty"`
	RefillInterval *string    `json:"refill_interval,omitempty"`
	Meta           model.JSON `json:"meta,omitempty"`
	CreatedAt      string     `json:"created_at"`
	RevokedAt      *string    `json:"revoked_at,omitempty"`
}

// Create handles POST /v1/credentials.
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

	if req.BaseURL == "" || req.AuthScheme == "" || req.APIKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "base_url, auth_scheme, and api_key are required"})
		return
	}

	validSchemes := map[string]bool{"bearer": true, "x-api-key": true, "api-key": true, "query_param": true}
	if !validSchemes[req.AuthScheme] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid auth_scheme"})
		return
	}

	// SSRF hardening: validate BaseURL before storing
	if err := proxy.ValidateBaseURL(req.BaseURL); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid base_url: %v", err)})
		return
	}

	// Generate DEK, encrypt the API key, wrap the DEK via KMS
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

	// Zero plaintext DEK and API key
	for i := range dek {
		dek[i] = 0
	}

	// Validate refill_interval if provided
	if req.RefillInterval != nil {
		if _, err := time.ParseDuration(*req.RefillInterval); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid refill_interval: must be a valid Go duration (e.g. 1h, 24h)"})
			return
		}
	}

	// Resolve identity: explicit identity_id or auto-upsert via external_id
	var identityID *uuid.UUID
	if req.IdentityID != nil {
		id, err := uuid.Parse(*req.IdentityID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid identity_id"})
			return
		}
		var ident model.Identity
		if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&ident).Error; err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "identity not found"})
			return
		}
		identityID = &id
	} else if req.ExternalID != nil && *req.ExternalID != "" {
		var ident model.Identity
		err := h.db.Where("external_id = ? AND org_id = ?", *req.ExternalID, org.ID).First(&ident).Error
		if err == gorm.ErrRecordNotFound {
			ident = model.Identity{
				ID:         uuid.New(),
				OrgID:      org.ID,
				ExternalID: *req.ExternalID,
			}
			if err := h.db.Create(&ident).Error; err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create identity"})
				return
			}
		} else if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve identity"})
			return
		}
		identityID = &ident.ID
	}

	// Auto-detect provider from base_url
	var providerID string
	if p, ok := registry.Global().MatchByBaseURL(req.BaseURL); ok {
		providerID = p.ID
	}

	cred := model.Credential{
		ID:             uuid.New(),
		OrgID:          org.ID,
		Label:          req.Label,
		BaseURL:        req.BaseURL,
		AuthScheme:     req.AuthScheme,
		ProviderID:     providerID,
		IdentityID:     identityID,
		EncryptedKey:   encryptedKey,
		WrappedDEK:     wrappedDEK,
		Remaining:      req.Remaining,
		RefillAmount:   req.RefillAmount,
		RefillInterval: req.RefillInterval,
		Meta:           req.Meta,
	}

	if err := h.db.Create(&cred).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store credential"})
		return
	}

	// Seed Redis counter if a cap is configured
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
	if cred.IdentityID != nil {
		s := cred.IdentityID.String()
		credResp.IdentityID = &s
	}
	writeJSON(w, http.StatusCreated, credResp)
}

// List handles GET /v1/credentials.
// Supports query params: ?identity_id=, ?external_id=, ?meta={"key":"value"}
func (h *CredentialHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	q := h.db.Where("credentials.org_id = ?", org.ID)

	if identityID := r.URL.Query().Get("identity_id"); identityID != "" {
		q = q.Where("identity_id = ?", identityID)
	}
	if externalID := r.URL.Query().Get("external_id"); externalID != "" {
		q = q.Joins("JOIN identities ON identities.id = credentials.identity_id").
			Where("identities.external_id = ? AND identities.org_id = ?", externalID, org.ID)
	}
	if metaFilter := r.URL.Query().Get("meta"); metaFilter != "" {
		q = q.Where("credentials.meta @> ?::jsonb", metaFilter)
	}

	var creds []model.Credential
	if err := q.Find(&creds).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list credentials"})
		return
	}

	resp := make([]credentialResponse, len(creds))
	for i, c := range creds {
		resp[i] = credentialResponse{
			ID:             c.ID.String(),
			Label:          c.Label,
			BaseURL:        c.BaseURL,
			AuthScheme:     c.AuthScheme,
			ProviderID:     c.ProviderID,
			Remaining:      c.Remaining,
			RefillAmount:   c.RefillAmount,
			RefillInterval: c.RefillInterval,
			Meta:           c.Meta,
			CreatedAt:      c.CreatedAt.Format(time.RFC3339),
		}
		if c.IdentityID != nil {
			s := c.IdentityID.String()
			resp[i].IdentityID = &s
		}
		if c.RevokedAt != nil {
			s := c.RevokedAt.Format(time.RFC3339)
			resp[i].RevokedAt = &s
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// Revoke handles DELETE /v1/credentials/{id}.
func (h *CredentialHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	credID := chi.URLParam(r, "id")
	if credID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential id required"})
		return
	}

	now := time.Now()
	result := h.db.Model(&model.Credential{}).
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", credID, org.ID).
		Update("revoked_at", &now)

	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "credential not found or already revoked"})
		return
	}

	// Invalidate all cache tiers
	_ = h.cacheManager.InvalidateCredential(r.Context(), credID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
