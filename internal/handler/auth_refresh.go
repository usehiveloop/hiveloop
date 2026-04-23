package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// Refresh handles POST /auth/refresh.
//
// The rotation flow is wrapped in a transaction with SELECT ... FOR UPDATE so
// two concurrent requests presenting the same token cannot both pass the
// revoked_at check. If a caller presents a token that has already been marked
// revoked (reuse detection) the whole refresh chain for that user is
// invalidated — a signal of probable token theft.
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

	tokenHash := hashToken(req.RefreshToken)

	var storedToken model.RefreshToken
	var reuseDetected bool

	txErr := h.db.Transaction(func(tx *gorm.DB) error {
		// Row-lock the token so two concurrent refreshes serialize. The
		// first commits the revoked_at update; the second re-reads the now
		// non-null revoked_at and triggers reuse detection below.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("token_hash = ?", tokenHash).
			First(&storedToken).Error; err != nil {
			return err
		}

		if storedToken.RevokedAt != nil {
			// The token exists but is already revoked: either a replay of a
			// previously rotated token or a genuine reuse. Either way we
			// invalidate every outstanding refresh token for this user.
			reuseDetected = true
			now := time.Now()
			if err := tx.Model(&model.RefreshToken{}).
				Where("user_id = ? AND revoked_at IS NULL", storedToken.UserID).
				Update("revoked_at", &now).Error; err != nil {
				return err
			}
			return nil
		}

		if time.Now().After(storedToken.ExpiresAt) {
			return errRefreshExpired
		}

		now := time.Now()
		return tx.Model(&storedToken).Update("revoked_at", &now).Error
	})

	if txErr != nil {
		if errors.Is(txErr, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "refresh token revoked or not found"})
			return
		}
		if errors.Is(txErr, errRefreshExpired) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "refresh token expired"})
			return
		}
		slog.Error("refresh rotation failed", "error", txErr)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if reuseDetected {
		slog.Warn("refresh token reuse detected; chain revoked",
			"user_id", storedToken.UserID, "token_hash", tokenHash)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "refresh token reuse detected"})
		return
	}

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

// errRefreshExpired is returned from the rotation transaction to signal a
// 401-expired response without ambiguous ErrRecordNotFound semantics.
var errRefreshExpired = errors.New("refresh token expired")

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

	claims, ok := middleware.AuthClaimsFromContext(r.Context())
	if !ok || claims == nil || claims.UserID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing auth context"})
		return
	}

	// Verify the supplied refresh token belongs to the caller before revoking.
	// Use the JWT claim as the primary check (cheap, no DB lookup needed
	// when tokens do not match) and cross-check the stored row to cover the
	// case where the token is valid but not recorded or already revoked.
	tokenUserID, _, err := auth.ValidateRefreshToken(h.signingKey, req.RefreshToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
		return
	}
	if tokenUserID != claims.UserID {
		slog.Warn("logout ownership mismatch",
			"auth_user_id", claims.UserID, "token_user_id", tokenUserID)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "refresh token does not belong to this user"})
		return
	}

	tokenHash := hashToken(req.RefreshToken)

	var stored model.RefreshToken
	if err := h.db.Where("token_hash = ?", tokenHash).First(&stored).Error; err == nil {
		if stored.UserID.String() != claims.UserID {
			slog.Warn("logout ownership mismatch on stored token",
				"auth_user_id", claims.UserID, "stored_user_id", stored.UserID)
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "refresh token does not belong to this user"})
			return
		}
	}

	now := time.Now()
	h.db.Model(&model.RefreshToken{}).
		Where("token_hash = ? AND user_id = ? AND revoked_at IS NULL", tokenHash, claims.UserID).
		Update("revoked_at", &now)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Me handles GET /auth/me.
// @Summary Get current user
// @Description Returns the current user and their organization memberships.
// @Tags auth
// @Produce json
// @Success 200 {object} meResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /auth/me [get]
