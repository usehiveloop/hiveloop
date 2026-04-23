package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"


	"github.com/usehiveloop/hiveloop/internal/model"
)

// oauthProfile holds the normalised user info fetched from an OAuth provider.

// provider (e.g. X/Twitter) does not return a user email.

// isPlaceholderEmail reports whether the email is a generated placeholder.
func (h *OAuthHandler) issueExchangeTokenAndRedirect(w http.ResponseWriter, r *http.Request, provider string, user *model.User) {
	plaintext, hash, err := model.GenerateExchangeToken()
	if err != nil {
		slog.Error("failed to generate exchange token", "error", err)
		h.redirectError(w, r, "internal_error")
		return
	}

	exchangeToken := model.OAuthExchangeToken{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	if err := h.db.Create(&exchangeToken).Error; err != nil {
		slog.Error("failed to store exchange token", "error", err)
		h.redirectError(w, r, "internal_error")
		return
	}

	redirectURL := fmt.Sprintf("%s/oauth/%s/callback?token=%s", h.frontendURL, provider, plaintext)
	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// ---------------------------------------------------------------------------
// Exchange endpoint — swap the one-time token for access + refresh tokens.
// ---------------------------------------------------------------------------

type exchangeRequest struct {
	Token string `json:"token"`
}

// Exchange handles POST /oauth/exchange.
// @Summary Exchange OAuth token for access and refresh tokens
// @Description Exchanges a short-lived, single-use OAuth exchange token for an access/refresh token pair. The exchange token is obtained from the OAuth callback redirect.
// @Tags oauth
// @Accept json
// @Produce json
// @Param body body exchangeRequest true "Exchange token"
// @Success 200 {object} authResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Router /oauth/exchange [post]
func (h *OAuthHandler) Exchange(w http.ResponseWriter, r *http.Request) {
	var req exchangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}

	tokenHash := model.HashExchangeToken(req.Token)

	var et model.OAuthExchangeToken
	err := h.db.Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", tokenHash, time.Now()).First(&et).Error
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
		return
	}

	// Mark as used.
	now := time.Now()
	if err := h.db.Model(&et).Update("used_at", &now).Error; err != nil {
		slog.Error("failed to mark exchange token as used", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Load user.
	var user model.User
	if err := h.db.Where("id = ?", et.UserID).First(&user).Error; err != nil {
		slog.Error("oauth exchange: user not found", "user_id", et.UserID, "error", err)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}

	// Load memberships.
	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", user.ID).Find(&memberships)

	if len(memberships) == 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization memberships"})
		return
	}

	orgID := memberships[0].OrgID.String()
	role := memberships[0].Role

	h.issueTokensAndRespond(w, http.StatusOK, user, orgID, role, memberships)
}

// ---------------------------------------------------------------------------
// Token issuance (mirrors AuthHandler.issueTokensAndRespond)
// ---------------------------------------------------------------------------

