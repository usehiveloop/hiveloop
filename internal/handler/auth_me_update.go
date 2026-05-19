package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// UpdateMe handles PATCH /auth/me.
// @Summary Update current user profile
// @Description Updates the authenticated user's name, avatar, and/or email. Email
//
//	changes require confirmation and are not applied immediately.
//
// @Tags auth
// @Accept json
// @Produce json
// @Param body body updateMeRequest true "Fields to update"
// @Success 200 {object} userResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Security BearerAuth
// @Router /auth/me [patch]
func (h *AuthHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req updateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updates := map[string]interface{}{}

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name cannot be empty"})
			return
		}
		updates["name"] = trimmed
	}

	if req.AvatarURL != nil {
		raw := strings.TrimSpace(*req.AvatarURL)
		if raw != "" {
			parsed, err := url.Parse(raw)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "avatar_url must be an absolute http(s) URL"})
				return
			}
		}
		updates["avatar_url"] = raw
	}

	if req.Email != nil {
		newEmail := strings.TrimSpace(strings.ToLower(*req.Email))
		if newEmail == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email cannot be empty"})
			return
		}
		if newEmail == user.Email {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "new email is the same as the current email"})
			return
		}

		var oauthCount int64
		h.db.Model(&model.OAuthAccount{}).Where("user_id = ?", user.ID).Count(&oauthCount)
		if oauthCount > 0 {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "email is managed by your linked OAuth account and cannot be changed"})
			return
		}

		var existing model.User
		if err := h.db.Where("email = ?", newEmail).First(&existing).Error; err == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email is already in use"})
			return
		}

		plainToken, tokenHash, err := model.GenerateVerificationToken()
		if err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to generate verification token", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}

		verification := model.EmailVerification{
			UserID:    user.ID,
			TokenHash: tokenHash,
			NewEmail:  &newEmail,
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		if err := h.db.Create(&verification).Error; err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to store verification token", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}

		confirmURL := fmt.Sprintf("%s/auth/confirm-email?token=%s", h.frontendURL, plainToken)
		_ = h.emailSender.SendTemplate(r.Context(), email.TemplateMessage{
			To:   newEmail,
			Slug: email.TmplAuthConfirmEmail,
			Variables: email.TemplateVars{
				"firstName":       firstNameFrom(*user),
				"email":           newEmail,
				"confirmationUrl": confirmURL,
				"expiresIn":       "24 hours",
			},
		})

		now := time.Now()
		_ = h.emailSender.SendTemplate(r.Context(), email.TemplateMessage{
			To:   user.Email,
			Slug: email.TmplAuthEmailChanged,
			Variables: email.TemplateVars{
				"firstName": firstNameFrom(*user),
				"oldEmail":  user.Email,
				"newEmail":  newEmail,
				"changedAt": now.Format("January 2, 2006 at 3:04 PM UTC"),
			},
		})

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "verification_sent",
			"message": fmt.Sprintf("Verification code sent to %s", newEmail),
			"user": userResponse{
				ID:             user.ID.String(),
				Email:          user.Email,
				Name:           user.Name,
				AvatarURL:      user.AvatarURL,
				EmailConfirmed: user.EmailConfirmedAt != nil,
			},
		})
		return
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	if err := h.db.Model(user).Updates(updates).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to update user", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if user.AvatarURL != nil && req.AvatarURL != nil {
		user.AvatarURL = req.AvatarURL
	}
	if req.Name != nil {
		user.Name = strings.TrimSpace(*req.Name)
	}

	writeJSON(w, http.StatusOK, userResponse{
		ID:             user.ID.String(),
		Email:          user.Email,
		Name:           user.Name,
		AvatarURL:      user.AvatarURL,
		EmailConfirmed: user.EmailConfirmedAt != nil,
	})
}

// ConfirmEmailChange handles POST /auth/me/confirm-email.
// @Summary Confirm email change
// @Description Confirms a pending email change using a verification token.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body confirmEmailChangeRequest true "Confirmation token"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /auth/me/confirm-email [post]
func (h *AuthHandler) ConfirmEmailChange(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req confirmEmailChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}

	tokenHash := model.HashVerificationToken(req.Token)

	var verification model.EmailVerification
	if err := h.db.Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", tokenHash, time.Now()).First(&verification).Error; err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired token"})
		return
	}

	if verification.UserID != user.ID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "token does not belong to this user"})
		return
	}

	if verification.NewEmail == nil || *verification.NewEmail == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "this token is not for an email change"})
		return
	}

	newEmail := *verification.NewEmail

	var existing model.User
	if err := h.db.Where("email = ? AND id != ?", newEmail, user.ID).First(&existing).Error; err == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email is already in use"})
		return
	}

	now := time.Now()
	if err := h.db.Model(user).Updates(map[string]interface{}{
		"email":              newEmail,
		"email_confirmed_at": now,
	}).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to update email", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	h.db.Model(&verification).Update("used_at", &now)

	logging.FromContext(r.Context()).InfoContext(r.Context(), "email changed", "user_id", user.ID, "old_email", user.Email, "new_email", newEmail)
	writeJSON(w, http.StatusOK, map[string]string{"status": "confirmed", "message": "Email address updated successfully"})
}
