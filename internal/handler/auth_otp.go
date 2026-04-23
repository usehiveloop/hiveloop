package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const (
	// Request path: per-email throttle to prevent email bombing. Mirrors the
	// pattern used by ForgotPassword (3 per 15 minutes).
	otpRequestMaxPerWindow = 3
	otpRequestWindow       = 15 * time.Minute

	// Verify path: per-email brute-force lockout. After this many failed
	// verify attempts within otpVerifyWindow the email is locked for
	// otpVerifyLockout. A 6-digit code has only 1,000,000 combinations so
	// we must be aggressive here.
	otpVerifyMaxFailures = 5
	otpVerifyWindow      = 10 * time.Minute
	otpVerifyLockout     = 15 * time.Minute
)

func (h *AuthHandler) OTPRequest(w http.ResponseWriter, r *http.Request) {
	// Generic body returned for both success and rate-limited responses so
	// that an attacker can't use response content to enumerate emails or
	// infer throttle state. Only the status code / Retry-After header differs.
	genericBody := map[string]string{"status": "ok"}

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

	// Per-email throttle to prevent email bombing. We count OTPCode rows
	// created for this email within the window; we do NOT look up the user
	// first, so this runs identically for existing and non-existing emails
	// (anti-enumeration). Limit mirrors ForgotPassword: 3 per 15 minutes.
	var recentCount int64
	cutoff := time.Now().Add(-otpRequestWindow)
	h.db.Model(&model.OTPCode{}).
		Where("email = ? AND created_at > ?", req.Email, cutoff).
		Count(&recentCount)
	if recentCount >= otpRequestMaxPerWindow {
		var oldest model.OTPCode
		retryAfter := int(otpRequestWindow.Seconds())
		if err := h.db.Where("email = ? AND created_at > ?", req.Email, cutoff).
			Order("created_at ASC").
			First(&oldest).Error; err == nil {
			remaining := time.Until(oldest.CreatedAt.Add(otpRequestWindow))
			if remaining > 0 {
				retryAfter = int(remaining.Seconds()) + 1
			}
		}
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		writeJSON(w, http.StatusTooManyRequests, genericBody)
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

	writeJSON(w, http.StatusOK, genericBody)
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
// @Failure 429 {object} errorResponse
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

	// Per-email lockout to prevent brute-force of the 6-digit code space.
	// A 6-digit OTP has only 1,000,000 possible values; without per-email
	// tracking the global per-IP rate limit is insufficient (see #60).
	if locked, retryAfter := h.isOTPVerifyLocked(req.Email); locked {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many failed attempts, please try again later"})
		return
	}

	// Look up the OTP by hash
	codeHash := model.HashOTPCode(req.Code)
	var otp model.OTPCode
	err := h.db.Where("email = ? AND token_hash = ? AND used_at IS NULL", req.Email, codeHash).First(&otp).Error
	if err != nil {
		h.recordOTPVerifyFailureAndRotate(req.Email)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired code"})
		return
	}

	if time.Now().After(otp.ExpiresAt) {
		h.recordOTPVerifyFailureAndRotate(req.Email)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired code"})
		return
	}

	// Mark code as used
	now := time.Now()
	h.db.Model(&otp).Update("used_at", &now)

	h.clearOTPVerifyFailures(req.Email)

	// Find or create user
	var user model.User
	err = h.db.Where("email = ?", req.Email).First(&user).Error
	if err != nil {
		// New user — create account with org in a transaction
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

			org = model.Org{
				Name: fmt.Sprintf("%s's Workspace", user.Name),
			}
			if err := tx.Create(&org).Error; err != nil {
				return fmt.Errorf("creating org: %w", err)
			}

			membership := model.OrgMembership{
				UserID: user.ID,
				OrgID:  org.ID,
				Role:   "owner",
			}
			if err := tx.Create(&membership).Error; err != nil {
				return fmt.Errorf("creating membership: %w", err)
			}

			return nil
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

// isOTPVerifyLocked reports whether this email is currently locked out due
// to too many failed verify attempts. Returns the Retry-After seconds when
// locked.
func (h *AuthHandler) isOTPVerifyLocked(email string) (bool, int) {
	h.otpVerifyMu.Lock()
	defer h.otpVerifyMu.Unlock()
	a, ok := h.otpVerifyAttempts[email]
	if !ok {
		return false, 0
	}
	if time.Since(a.firstAt) > otpVerifyLockout {
		delete(h.otpVerifyAttempts, email)
		return false, 0
	}
	if a.failures >= otpVerifyMaxFailures {
		remaining := time.Until(a.firstAt.Add(otpVerifyLockout))
		if remaining < 0 {
			remaining = 0
		}
		return true, int(remaining.Seconds()) + 1
	}
	return false, 0
}

// recordOTPVerifyFailureAndRotate bumps the failure counter for this email
// and invalidates any outstanding OTP code so a leaked code can't be used
// later. Rotating on every failure means an attacker brute-forcing the
// code space burns the victim's real code as collateral damage, but more
// importantly it ensures that once we start seeing failures the code in
// flight is no longer valid.
func (h *AuthHandler) recordOTPVerifyFailureAndRotate(email string) {
	h.otpVerifyMu.Lock()
	a, ok := h.otpVerifyAttempts[email]
	if !ok || time.Since(a.firstAt) > otpVerifyWindow {
		h.otpVerifyAttempts[email] = &loginAttempt{failures: 1, firstAt: time.Now()}
	} else {
		a.failures++
	}
	h.otpVerifyMu.Unlock()

	// Invalidate any unused OTP codes for this email so a leaked or
	// partially-guessed code is no longer usable. Best-effort: errors are
	// logged but do not block the response.
	if err := h.db.Model(&model.OTPCode{}).
		Where("email = ? AND used_at IS NULL", email).
		Update("used_at", time.Now()).Error; err != nil {
		slog.Warn("failed to rotate OTP after failed verify", "error", err, "email", email)
	}
}

func (h *AuthHandler) clearOTPVerifyFailures(email string) {
	h.otpVerifyMu.Lock()
	defer h.otpVerifyMu.Unlock()
	delete(h.otpVerifyAttempts, email)
}

// --- Helpers ---

// firstNameFrom returns a best-effort first name for email personalization.
// Falls back to the email local-part when the user hasn't provided a name,
// then to a generic "there" so templates don't render "Hi ,".
