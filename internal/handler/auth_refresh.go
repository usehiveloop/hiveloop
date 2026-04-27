package handler

import (
	"encoding/json"
	"net/http"
	"time"


	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// @Summary Refresh tokens
// @Description Exchanges a refresh token for new access and refresh tokens.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body refreshRequest true "Refresh parameters"
// @Success 200 {object} authResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Router /auth/refresh [post]
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.RefreshToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "refresh_token is required"})
		return
	}

	// Validate the refresh JWT.
	userID, _, err := auth.ValidateRefreshToken(h.signingKey, req.RefreshToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
		return
	}

	// Check the token hash in the database (revocation check + rotation).
	tokenHash := hashToken(req.RefreshToken)
	var storedToken model.RefreshToken
	if err := h.db.Where("token_hash = ? AND revoked_at IS NULL", tokenHash).First(&storedToken).Error; err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "refresh token revoked or not found"})
		return
	}

	if time.Now().After(storedToken.ExpiresAt) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "refresh token expired"})
		return
	}

	// Revoke the old refresh token (rotation).
	now := time.Now()
	h.db.Model(&storedToken).Update("revoked_at", &now)

	// Get memberships to determine org/role.
	var user model.User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}

	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", user.ID).Find(&memberships)

	if len(memberships) == 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization memberships"})
		return
	}

	orgID := memberships[0].OrgID.String()
	role := memberships[0].Role
	if req.OrgID != "" {
		found := false
		for _, m := range memberships {
			if m.OrgID.String() == req.OrgID {
				orgID = req.OrgID
				role = m.Role
				found = true
				break
			}
		}
		if !found {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of the requested organization"})
			return
		}
	}

	h.issueTokensAndRespond(w, http.StatusOK, user, orgID, role)
}

// Logout handles POST /auth/logout.
// @Summary Log out
// @Description Revokes a refresh token.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body logoutRequest true "Logout parameters"
// @Success 200 {object} statusResponse
// @Security BearerAuth
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.RefreshToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "refresh_token is required"})
		return
	}

	tokenHash := hashToken(req.RefreshToken)
	now := time.Now()
	h.db.Model(&model.RefreshToken{}).Where("token_hash = ? AND revoked_at IS NULL", tokenHash).Update("revoked_at", &now)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

