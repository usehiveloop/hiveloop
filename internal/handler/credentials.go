package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/useportal/proxy-bridge/internal/cache"
	"github.com/useportal/proxy-bridge/internal/crypto"
	"github.com/useportal/proxy-bridge/internal/middleware"
	"github.com/useportal/proxy-bridge/internal/model"
)

// CredentialHandler manages credential CRUD operations.
type CredentialHandler struct {
	db           *gorm.DB
	vault        *crypto.VaultTransit
	cacheManager *cache.Manager
}

// NewCredentialHandler creates a new credential handler.
func NewCredentialHandler(db *gorm.DB, vault *crypto.VaultTransit, cm *cache.Manager) *CredentialHandler {
	return &CredentialHandler{db: db, vault: vault, cacheManager: cm}
}

type createCredentialRequest struct {
	Label      string `json:"label"`
	BaseURL    string `json:"base_url"`
	AuthScheme string `json:"auth_scheme"`
	APIKey     string `json:"api_key"`
}

type credentialResponse struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	BaseURL    string  `json:"base_url"`
	AuthScheme string  `json:"auth_scheme"`
	CreatedAt  string  `json:"created_at"`
	RevokedAt  *string `json:"revoked_at,omitempty"`
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

	// Generate DEK, encrypt the API key, wrap the DEK via Vault Transit
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

	wrappedDEK, err := h.vault.Wrap(dek)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key wrapping failed"})
		return
	}

	// Zero plaintext DEK and API key
	for i := range dek {
		dek[i] = 0
	}

	cred := model.Credential{
		ID:           uuid.New(),
		OrgID:        org.ID,
		Label:        req.Label,
		BaseURL:      req.BaseURL,
		AuthScheme:   req.AuthScheme,
		EncryptedKey: encryptedKey,
		WrappedDEK:   wrappedDEK,
	}

	if err := h.db.Create(&cred).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store credential"})
		return
	}

	writeJSON(w, http.StatusCreated, credentialResponse{
		ID:         cred.ID.String(),
		Label:      cred.Label,
		BaseURL:    cred.BaseURL,
		AuthScheme: cred.AuthScheme,
		CreatedAt:  cred.CreatedAt.Format(time.RFC3339),
	})
}

// List handles GET /v1/credentials.
func (h *CredentialHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var creds []model.Credential
	if err := h.db.Where("org_id = ?", org.ID).Find(&creds).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list credentials"})
		return
	}

	resp := make([]credentialResponse, len(creds))
	for i, c := range creds {
		resp[i] = credentialResponse{
			ID:         c.ID.String(),
			Label:      c.Label,
			BaseURL:    c.BaseURL,
			AuthScheme: c.AuthScheme,
			CreatedAt:  c.CreatedAt.Format(time.RFC3339),
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
