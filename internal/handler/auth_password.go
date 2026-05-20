package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/email"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// @Summary Request password reset
// @Description Sends a password reset link to the email address if an account exists.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body forgotPasswordRequest true "Email address"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Router /auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	genericResponse := map[string]string{
		"status":  "ok",
		"message": "If an account with that email exists, a password reset link has been sent.",
	}

	var req forgotPasswordRequest
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

	var count int64
	cutoff := time.Now().Add(-15 * time.Minute)
	h.db.Model(&model.PasswordReset{}).Where("user_id = ? AND created_at > ?", user.ID, cutoff).Count(&count)
	if count >= 3 {
		writeJSON(w, http.StatusOK, genericResponse)
		return
	}

	plainToken, tokenHash, err := model.GenerateResetToken()
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to generate reset token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	reset := model.PasswordReset{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	if err := h.db.Create(&reset).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to store reset token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	resetURL := fmt.Sprintf("%s/auth/reset-password?token=%s", h.frontendURL, plainToken)
	_ = h.emailSender.SendTemplate(r.Context(), email.TemplateMessage{
		To:   user.Email,
		Slug: email.TmplAuthPasswordReset,
		Variables: email.TemplateVars{
			"firstName": firstNameFrom(user),
			"resetUrl":  resetURL,
			"expiresIn": "1 hour",
		},
	})

	logging.FromContext(r.Context()).InfoContext(r.Context(), "password reset requested", "email", user.Email)
	writeJSON(w, http.StatusOK, genericResponse)
}

// ResetPassword handles POST /auth/reset-password.
// @Summary Reset password
// @Description Resets a user's password using a reset token. Revokes all sessions.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body resetPasswordRequest true "Reset token and new password"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Router /auth/reset-password [post]
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
		return
	}
	if len(req.NewPassword) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}

	tokenHash := model.HashResetToken(req.Token)

	var reset model.PasswordReset
	if err := h.db.Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", tokenHash, time.Now()).First(&reset).Error; err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired token"})
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to hash password", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	now := time.Now()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&reset).Update("used_at", &now).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", reset.UserID).Update("password_hash", newHash).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.RefreshToken{}).
			Where("user_id = ? AND revoked_at IS NULL", reset.UserID).
			Update("revoked_at", &now).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.PasswordReset{}).
			Where("user_id = ? AND used_at IS NULL AND id != ?", reset.UserID, reset.ID).
			Update("used_at", &now).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to reset password", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	logging.FromContext(r.Context()).InfoContext(r.Context(), "password reset completed", "user_id", reset.UserID)

	var user model.User
	if err := h.db.Where("id = ?", reset.UserID).First(&user).Error; err == nil {
		_ = h.emailSender.SendTemplate(r.Context(), email.TemplateMessage{
			To:   user.Email,
			Slug: email.TmplAuthPasswordChanged,
			Variables: email.TemplateVars{
				"firstName": firstNameFrom(user),
				"changedAt": now.UTC().Format(time.RFC1123),
			},
		})
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Password has been reset. Please log in.",
	})
}

// ChangePassword handles POST /auth/change-password (authenticated).
// @Summary Change password
// @Description Changes the authenticated user's password. Revokes all sessions.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body changePasswordRequest true "Current and new password"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /auth/change-password [post]
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AuthClaimsFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "current_password and new_password are required"})
		return
	}
	if len(req.NewPassword) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}

	var user model.User
	if err := h.db.Where("id = ?", claims.UserID).First(&user).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	if user.PasswordHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password login not configured for this account"})
		return
	}

	if !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "current password is incorrect"})
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to hash password", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	now := time.Now()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("password_hash", newHash).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.RefreshToken{}).
			Where("user_id = ? AND revoked_at IS NULL", user.ID).
			Update("revoked_at", &now).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to change password", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	logging.FromContext(r.Context()).InfoContext(r.Context(), "password changed", "user_id", user.ID)

	_ = h.emailSender.SendTemplate(r.Context(), email.TemplateMessage{
		To:   user.Email,
		Slug: email.TmplAuthPasswordChanged,
		Variables: email.TemplateVars{
			"firstName": firstNameFrom(user),
			"changedAt": now.UTC().Format(time.RFC1123),
		},
	})

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Password changed. Please log in again.",
	})
}

const (
	maxLoginFailures   = 5
	loginLockoutWindow = 15 * time.Minute
)
