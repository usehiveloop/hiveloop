package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/mcp"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/token"
)

// TokenHandler manages sandbox proxy token operations.

// MCPServerCache is an interface for evicting cached MCP servers.

// NewTokenHandler creates a new token handler.
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

	resp := mintTokenResponse{
		Token:     "ptok_" + tokenStr,
		ExpiresAt: expiresAt.Format(time.RFC3339),
		JTI:       jti,
	}
	if len(req.Scopes) > 0 && h.mcpBaseURL != "" {
		ep := h.mcpBaseURL + "/" + jti
		resp.MCPEndpoint = &ep
	}
	writeJSON(w, http.StatusCreated, resp)
}

// tokenResponse is the API representation of a token record.
type tokenListItem struct {
	ID             string     `json:"id"`
	JTI            string     `json:"jti"`
	CredentialID   string     `json:"credential_id"`
	Remaining      *int64     `json:"remaining,omitempty"`
	RefillAmount   *int64     `json:"refill_amount,omitempty"`
	RefillInterval *string    `json:"refill_interval,omitempty"`
	Scopes         model.JSON `json:"scopes,omitempty"`
	Meta           model.JSON `json:"meta,omitempty"`
	ExpiresAt      string     `json:"expires_at"`
	RevokedAt      *string    `json:"revoked_at,omitempty"`
	CreatedAt      string     `json:"created_at"`
}

func toTokenListItem(t model.Token) tokenListItem {
	item := tokenListItem{
		ID:             t.ID.String(),
		JTI:            t.JTI,
		CredentialID:   t.CredentialID.String(),
		Remaining:      t.Remaining,
		RefillAmount:   t.RefillAmount,
		RefillInterval: t.RefillInterval,
		Scopes:         t.Scopes,
		Meta:           t.Meta,
		ExpiresAt:      t.ExpiresAt.Format(time.RFC3339),
		CreatedAt:      t.CreatedAt.Format(time.RFC3339),
	}
	if t.RevokedAt != nil {
		s := t.RevokedAt.Format(time.RFC3339)
		item.RevokedAt = &s
	}
	return item
}

// List handles GET /v1/tokens.
// @Summary List tokens
// @Description Returns tokens for the organization with cursor pagination. Supports filtering by credential_id.
// @Tags tokens
// @Produce json
// @Param limit query int false "Max items per page (1-100, default 50)"
// @Param cursor query string false "Pagination cursor from previous response"
// @Param credential_id query string false "Filter by credential ID"
// @Success 200 {object} paginatedResponse[tokenListItem]
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/tokens [get]
