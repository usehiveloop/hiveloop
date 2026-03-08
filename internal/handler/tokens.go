package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/useportal/proxy-bridge/internal/cache"
	"github.com/useportal/proxy-bridge/internal/middleware"
	"github.com/useportal/proxy-bridge/internal/model"
	"github.com/useportal/proxy-bridge/internal/token"
)

// TokenHandler manages sandbox proxy token operations.
type TokenHandler struct {
	db           *gorm.DB
	signingKey   []byte
	cacheManager *cache.Manager
}

// NewTokenHandler creates a new token handler.
func NewTokenHandler(db *gorm.DB, signingKey []byte, cm *cache.Manager) *TokenHandler {
	return &TokenHandler{db: db, signingKey: signingKey, cacheManager: cm}
}

type mintTokenRequest struct {
	CredentialID string `json:"credential_id"`
	TTL          string `json:"ttl"` // e.g. "1h", "24h"
}

type mintTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

const maxTokenTTL = 24 * time.Hour

// Mint handles POST /v1/tokens.
func (h *TokenHandler) Mint(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	var req mintTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.CredentialID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credential_id is required"})
		return
	}

	ttl := time.Hour // default
	if req.TTL != "" {
		var err error
		ttl, err = time.ParseDuration(req.TTL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ttl format"})
			return
		}
	}
	if ttl > maxTokenTTL {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ttl exceeds maximum of 24h"})
		return
	}

	// Verify the credential exists and belongs to this org
	credUUID, err := uuid.Parse(req.CredentialID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid credential_id"})
		return
	}

	var cred model.Credential
	if err := h.db.Where("id = ? AND org_id = ? AND revoked_at IS NULL", credUUID, org.ID).First(&cred).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "credential not found or revoked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify credential"})
		return
	}

	// Mint the JWT
	tokenStr, jti, err := token.Mint(h.signingKey, org.ID.String(), cred.ID.String(), ttl)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to mint token"})
		return
	}

	expiresAt := time.Now().Add(ttl)
	tokenRecord := model.Token{
		ID:           uuid.New(),
		OrgID:        org.ID,
		CredentialID: cred.ID,
		JTI:          jti,
		ExpiresAt:    expiresAt,
	}
	if err := h.db.Create(&tokenRecord).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store token"})
		return
	}

	writeJSON(w, http.StatusCreated, mintTokenResponse{
		Token:     "ptok_" + tokenStr,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	})
}

// Revoke handles DELETE /v1/tokens/{jti}.
func (h *TokenHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	jti := chi.URLParam(r, "jti")
	if jti == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "jti required"})
		return
	}

	now := time.Now()
	result := h.db.Model(&model.Token{}).
		Where("jti = ? AND org_id = ? AND revoked_at IS NULL", jti, org.ID).
		Update("revoked_at", &now)

	if result.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke token"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found or already revoked"})
		return
	}

	// Propagate revocation through cache tiers
	_ = h.cacheManager.InvalidateToken(r.Context(), jti, 24*time.Hour)

	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
