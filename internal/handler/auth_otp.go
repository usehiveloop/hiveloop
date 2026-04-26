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
func (h *AuthHandler) OTPRequest(w http.ResponseWriter, r *http.Request) {
	var req otpRequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email is required"})
		return
	}

	// Admin mode: reject non-admin emails
	if h.adminMode && !h.platformAdminEmails[req.Email] {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
		return
	}

	// Invalidate any unused OTP codes for this email
	h.db.Model(&model.OTPCode{}).
		Where("email = ? AND used_at IS NULL", req.Email).
		Update("used_at", time.Now())

	// Generate new code
	plainCode, codeHash, err := model.GenerateOTPCode()
	if err != nil {
		slog.Error("failed to generate OTP code", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	otp := model.OTPCode{
		Email:     req.Email,
		TokenHash: codeHash,
		ExpiresAt: time.Now().Add(otpExpiry),
	}
	if err := h.db.Create(&otp).Error; err != nil {
		slog.Error("failed to store OTP code", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Send the code via email (asynq-queued, Kibamail-delivered, retried on failure).
	if err := h.emailSender.SendTemplate(r.Context(), email.TemplateMessage{
		To:   req.Email,
		Slug: email.TmplAuthOtpLogin,
		Variables: email.TemplateVars{
			"code":      plainCode,
			"email":     req.Email,
			"expiresIn": "10 minutes",
		},
	}); err != nil {
		// Enqueue failure shouldn't break login — user can request a new code.
		// Log loudly so it's paged on dashboards.
		slog.Error("failed to enqueue OTP email", "error", err, "email", req.Email)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// OTPVerify handles POST /auth/otp/verify.
// @Summary Verify an OTP code
// @Description Verifies the 6-digit code and returns access/refresh tokens. Creates the user account if it doesn't exist.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body otpVerifyPayload true "OTP verification"
// @Success 200 {object} authResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Router /auth/otp/verify [post]
func (h *AuthHandler) OTPVerify(w http.ResponseWriter, r *http.Request) {
	var req otpVerifyPayload
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

	// Look up the OTP by hash
	codeHash := model.HashOTPCode(req.Code)
	var otp model.OTPCode
	err := h.db.Where("email = ? AND token_hash = ? AND used_at IS NULL", req.Email, codeHash).First(&otp).Error
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired code"})
		return
	}

	if time.Now().After(otp.ExpiresAt) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired code"})
		return
	}

	// Mark code as used
	now := time.Now()
	h.db.Model(&otp).Update("used_at", &now)

	// Find or create user
	var user model.User
	err = h.db.Where("email = ?", req.Email).First(&user).Error
	if err != nil {
		// New user — create account with org and welcome credit grant in one tx.
		var org model.Org
		txErr := h.db.Transaction(func(tx *gorm.DB) error {
			user = model.User{
				Email:            req.Email,
				Name:             strings.Split(req.Email, "@")[0],
				EmailConfirmedAt: &now,
			}
			if err := tx.Create(&user).Error; err != nil {
				return fmt.Errorf("creating user: %w", err)
			}

			var orgErr error
			org, orgErr = createUserDefaultOrg(tx, h.credits, &user)
			return orgErr
		})
		if txErr != nil {
			slog.Error("failed to create user via OTP", "error", txErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create account"})
			return
		}

		slog.Info("user created via OTP", "user_id", user.ID, "email", user.Email)
		h.issueTokensAndRespond(w, http.StatusCreated, user, org.ID.String(), "owner")
		return
	}

	// Existing user — ensure email is confirmed
	if user.EmailConfirmedAt == nil {
		h.db.Model(&user).Update("email_confirmed_at", &now)
	}

	// Admin mode check
	if h.adminMode && !h.platformAdminEmails[user.Email] {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
		return
	}

	// Get memberships and issue tokens
	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", user.ID).Find(&memberships)

	if len(memberships) == 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization memberships"})
		return
	}

	slog.Info("user logged in via OTP", "user_id", user.ID, "email", user.Email)
	h.issueTokensAndRespond(w, http.StatusOK, user, memberships[0].OrgID.String(), memberships[0].Role)
}

// --- Helpers ---

// firstNameFrom returns a best-effort first name for email personalization.
// Falls back to the email local-part when the user hasn't provided a name,
// then to a generic "there" so templates don't render "Hi ,".
