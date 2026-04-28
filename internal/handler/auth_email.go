package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// @Summary Confirm email address
// @Description Confirms a user's email address using a verification token.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body confirmEmailRequest true "Confirmation token"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Router /auth/confirm-email [post]
func (h *AuthHandler) ConfirmEmail(w http.ResponseWriter, r *http.Request) {
	var req confirmEmailRequest
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

	// Load the user so we can determine whether this is the FIRST
	// confirmation (used below to gate the welcome email). Re-confirmations
	// (after an email change, for example) should not resend welcome.
	var user model.User
	if err := h.db.Where("id = ?", verification.UserID).First(&user).Error; err != nil {
		slog.Error("failed to load user for confirmation", "error", err, "user_id", verification.UserID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	isFirstConfirmation := user.EmailConfirmedAt == nil

	now := time.Now()
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&verification).Update("used_at", &now).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", verification.UserID).Update("email_confirmed_at", &now).Error; err != nil {
			return err
		}
		// Invalidate all other pending verification tokens for this user.
		if err := tx.Model(&model.EmailVerification{}).
			Where("user_id = ? AND used_at IS NULL AND id != ?", verification.UserID, verification.ID).
			Update("used_at", &now).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		slog.Error("failed to confirm email", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if isFirstConfirmation {
		_ = h.emailSender.SendTemplate(r.Context(), email.TemplateMessage{
			To:   user.Email,
			Slug: email.TmplAuthWelcome,
			Variables: email.TemplateVars{
				"firstName": firstNameFrom(user),
			},
		})
	}

	slog.Info("email confirmed", "user_id", verification.UserID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "confirmed"})
}

// ResendConfirmation handles POST /auth/resend-confirmation.
// @Summary Resend confirmation email
// @Description Sends a new email confirmation link. Rate limited to 1 per 60 seconds.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body resendConfirmationRequest true "Email address"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 429 {object} errorResponse
// @Router /auth/resend-confirmation [post]
func (h *AuthHandler) ResendConfirmation(w http.ResponseWriter, r *http.Request) {
	genericResponse := map[string]string{
		"status":  "ok",
		"message": "If the email exists and is unconfirmed, a confirmation email has been sent.",
	}

	var req resendConfirmationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email is required"})
		return
	}

	// Look up user. Return generic response if not found or already confirmed.
	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		writeJSON(w, http.StatusOK, genericResponse)
		return
	}
	if user.EmailConfirmedAt != nil {
		writeJSON(w, http.StatusOK, genericResponse)
		return
	}

	// Rate limit: max 1 per user per 60 seconds.
	var recent model.EmailVerification
	cutoff := time.Now().Add(-60 * time.Second)
	if err := h.db.Where("user_id = ? AND created_at > ?", user.ID, cutoff).First(&recent).Error; err == nil {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "please wait before requesting another confirmation email"})
		return
	}

	plainToken, tokenHash, err := model.GenerateVerificationToken()
	if err != nil {
		slog.Error("failed to generate verification token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	verification := model.EmailVerification{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := h.db.Create(&verification).Error; err != nil {
		slog.Error("failed to store verification token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	confirmURL := fmt.Sprintf("%s/auth/confirm-email?token=%s", h.frontendURL, plainToken)
	_ = h.emailSender.SendTemplate(r.Context(), email.TemplateMessage{
		To:   user.Email,
		Slug: email.TmplAuthConfirmEmail,
		Variables: email.TemplateVars{
			"firstName":       firstNameFrom(user),
			"email":           user.Email,
			"confirmationUrl": confirmURL,
			"expiresIn":       "24 hours",
		},
	})

	writeJSON(w, http.StatusOK, genericResponse)
}

