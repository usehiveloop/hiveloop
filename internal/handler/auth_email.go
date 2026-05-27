package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/email"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// @Summary Confirm email address
// @Description Confirms a user's email address using a 6-digit code.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body confirmEmailRequest true "Email and 6-digit code"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Router /auth/confirm-email [post]
func (h *AuthHandler) ConfirmEmail(w http.ResponseWriter, r *http.Request) {
	var req confirmEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Code = strings.TrimSpace(req.Code)

	if req.Email == "" || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and code are required"})
		return
	}

	codeHash := model.HashOTPCode(req.Code)
	var otp model.OTPCode
	if err := h.db.Where("email = ? AND token_hash = ? AND used_at IS NULL", req.Email, codeHash).First(&otp).Error; err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired code"})
		return
	}
	if time.Now().After(otp.ExpiresAt) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired code"})
		return
	}

	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to load user for confirmation", "error", err, "email", req.Email)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	isFirstConfirmation := user.EmailConfirmedAt == nil

	now := time.Now()
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&otp).Update("used_at", &now).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", user.ID).Update("email_confirmed_at", &now).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.OTPCode{}).
			Where("email = ? AND used_at IS NULL AND id != ?", req.Email, otp.ID).
			Update("used_at", &now).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to confirm email", "error", err)
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

	logging.FromContext(r.Context()).InfoContext(r.Context(), "email confirmed", "user_id", user.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "confirmed"})
}

// ResendConfirmation handles POST /auth/resend-confirmation.
// @Summary Resend confirmation email
// @Description Sends a new 6-digit email confirmation code. Rate limited to 1 per 60 seconds.
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

	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		writeJSON(w, http.StatusOK, genericResponse)
		return
	}
	if user.EmailConfirmedAt != nil {
		writeJSON(w, http.StatusOK, genericResponse)
		return
	}

	var recent model.OTPCode
	cutoff := time.Now().Add(-60 * time.Second)
	if err := h.db.Where("email = ? AND created_at > ?", user.Email, cutoff).First(&recent).Error; err == nil {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "please wait before requesting another confirmation email"})
		return
	}

	if err := h.sendEmailConfirmationCode(r.Context(), user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, genericResponse)
}

func (h *AuthHandler) sendEmailConfirmationCode(ctx context.Context, user model.User) error {
	now := time.Now()
	if err := h.db.Model(&model.OTPCode{}).
		Where("email = ? AND used_at IS NULL", user.Email).
		Update("used_at", now).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to invalidate existing confirmation codes", "error", err, "email", user.Email)
		return err
	}

	plainCode, codeHash, err := model.GenerateOTPCode()
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to generate confirmation code", "error", err)
		return err
	}

	otp := model.OTPCode{
		Email:     user.Email,
		TokenHash: codeHash,
		ExpiresAt: time.Now().Add(otpExpiry),
	}
	if err := h.db.Create(&otp).Error; err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to store confirmation code", "error", err)
		return err
	}

	if err := h.emailSender.SendTemplate(ctx, email.TemplateMessage{
		To:   user.Email,
		Slug: email.TmplAuthConfirmEmail,
		Variables: email.TemplateVars{
			"code":      plainCode,
			"email":     user.Email,
			"expiresIn": "10 minutes",
			"firstName": firstNameFrom(user),
		},
	}); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "failed to enqueue confirmation email", "error", err, "email", user.Email)
	}
	return nil
}

func (h *AuthHandler) resendConfirmationInBackground(user model.User) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := h.sendEmailConfirmationCode(ctx, user); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "auto-resend confirmation on login failed", "error", err, "email", user.Email)
	}
}
