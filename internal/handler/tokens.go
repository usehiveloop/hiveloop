package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/useportal/llmvault/internal/cache"
	"github.com/useportal/llmvault/internal/counter"
	"github.com/useportal/llmvault/internal/mcp"
	"github.com/useportal/llmvault/internal/mcp/catalog"
	"github.com/useportal/llmvault/internal/middleware"
	"github.com/useportal/llmvault/internal/model"
	"github.com/useportal/llmvault/internal/token"
)

// TokenHandler manages sandbox proxy token operations.
type TokenHandler struct {
	db           *gorm.DB
	signingKey   []byte
	cacheManager *cache.Manager
	counter      *counter.Counter
	catalog      *catalog.Catalog
}

// NewTokenHandler creates a new token handler.
func NewTokenHandler(db *gorm.DB, signingKey []byte, cm *cache.Manager, ctr *counter.Counter, cat *catalog.Catalog) *TokenHandler {
	return &TokenHandler{db: db, signingKey: signingKey, cacheManager: cm, counter: ctr, catalog: cat}
}

type mintTokenRequest struct {
	CredentialID   string           `json:"credential_id"`
	TTL            string           `json:"ttl"` // e.g. "1h", "24h"
	Remaining      *int64           `json:"remaining,omitempty"`
	RefillAmount   *int64           `json:"refill_amount,omitempty"`
	RefillInterval *string          `json:"refill_interval,omitempty"`
	Scopes         []mcp.TokenScope `json:"scopes,omitempty"`
	Meta           model.JSON       `json:"meta,omitempty"`
}

type mintTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	JTI       string `json:"jti"`
}

const maxTokenTTL = 24 * time.Hour

// Mint handles POST /v1/tokens.
// @Summary Mint a proxy token
// @Description Creates a short-lived JWT proxy token scoped to a credential.
// @Tags tokens
// @Accept json
// @Produce json
// @Param body body mintTokenRequest true "Token minting parameters"
// @Success 201 {object} mintTokenResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/tokens [post]
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

	// Validate refill_interval if provided
	if req.RefillInterval != nil {
		if _, err := time.ParseDuration(*req.RefillInterval); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid refill_interval: must be a valid Go duration (e.g. 1h, 24h)"})
			return
		}
	}

	// Validate scopes against catalog and database
	if len(req.Scopes) > 0 && h.catalog != nil {
		if err := mcp.ValidateScopes(h.db, org.ID, h.catalog, req.Scopes); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}

	// Compute scope hash for JWT claims
	var mintOpts []token.MintOptions
	var scopesJSON model.JSON
	if len(req.Scopes) > 0 {
		scopeHash, err := mcp.ScopeHash(req.Scopes)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to compute scope hash"})
			return
		}
		mintOpts = append(mintOpts, token.MintOptions{ScopeHash: scopeHash})

		// Serialize scopes to JSON for storage
		scopeBytes, err := json.Marshal(req.Scopes)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to serialize scopes"})
			return
		}
		var scopeMap model.JSON
		if err := json.Unmarshal(scopeBytes, &scopeMap); err != nil {
			// Scopes is an array, store it under a "scopes" key
			scopesJSON = model.JSON{"scopes": req.Scopes}
		} else {
			scopesJSON = scopeMap
		}
	}

	// Mint the JWT
	tokenStr, jti, err := token.Mint(h.signingKey, org.ID.String(), cred.ID.String(), ttl, mintOpts...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to mint token"})
		return
	}

	expiresAt := time.Now().Add(ttl)
	tokenRecord := model.Token{
		ID:             uuid.New(),
		OrgID:          org.ID,
		CredentialID:   cred.ID,
		JTI:            jti,
		ExpiresAt:      expiresAt,
		Remaining:      req.Remaining,
		RefillAmount:   req.RefillAmount,
		RefillInterval: req.RefillInterval,
		Scopes:         scopesJSON,
		Meta:           req.Meta,
	}
	if err := h.db.Create(&tokenRecord).Error; err != nil {
		slog.Error("failed to store token", "error", err, "org_id", org.ID, "credential_id", req.CredentialID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store token"})
		return
	}

	// Seed Redis counter if a cap is configured
	if tokenRecord.Remaining != nil && h.counter != nil {
		_ = h.counter.SeedToken(r.Context(), jti, *tokenRecord.Remaining, ttl)
	}

	slog.Info("token minted", "org_id", org.ID, "credential_id", req.CredentialID, "jti", jti, "ttl", ttl.String(), "scopes", len(req.Scopes))
	writeJSON(w, http.StatusCreated, mintTokenResponse{
		Token:     "ptok_" + tokenStr,
		ExpiresAt: expiresAt.Format(time.RFC3339),
		JTI:       jti,
	})
}

// Revoke handles DELETE /v1/tokens/{jti}.
// @Summary Revoke a proxy token
// @Description Revokes a proxy token by its JTI and propagates through cache tiers.
// @Tags tokens
// @Produce json
// @Param jti path string true "Token JTI"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/tokens/{jti} [delete]
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
		slog.Error("failed to revoke token", "error", result.Error, "org_id", org.ID, "jti", jti)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke token"})
		return
	}
	if result.RowsAffected == 0 {
		slog.Warn("token not found or already revoked", "org_id", org.ID, "jti", jti)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found or already revoked"})
		return
	}

	// Propagate revocation through cache tiers
	_ = h.cacheManager.InvalidateToken(r.Context(), jti, 24*time.Hour)

	slog.Info("token revoked", "org_id", org.ID, "jti", jti)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
