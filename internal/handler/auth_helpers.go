package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"


	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/model"
)
func (h *AuthHandler) isLoginLocked(email string) bool {
	h.loginMu.Lock()
	defer h.loginMu.Unlock()
	a, ok := h.loginAttempts[email]
	if !ok {
		return false
	}
	if time.Since(a.firstAt) > loginLockoutWindow {
		delete(h.loginAttempts, email)
		return false
	}
	return a.failures >= maxLoginFailures
}

func (h *AuthHandler) recordLoginFailure(email string) {
	h.loginMu.Lock()
	defer h.loginMu.Unlock()
	a, ok := h.loginAttempts[email]
	if !ok || time.Since(a.firstAt) > loginLockoutWindow {
		h.loginAttempts[email] = &loginAttempt{failures: 1, firstAt: time.Now()}
		return
	}
	a.failures++
}

func (h *AuthHandler) clearLoginFailures(email string) {
	h.loginMu.Lock()
	defer h.loginMu.Unlock()
	delete(h.loginAttempts, email)
}

// --- OTP Authentication ---

type otpRequestPayload struct {
	Email string `json:"email"`
}

type otpVerifyPayload struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

const otpExpiry = 10 * time.Minute

// OTPRequest handles POST /auth/otp/request.
// @Summary Request an OTP code
// @Description Sends a 6-digit one-time code to the given email address.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body otpRequestPayload true "OTP request"
// @Success 200 {object} map[string]string
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Router /auth/otp/request [post]
func firstNameFrom(user model.User) string {
	if name := strings.TrimSpace(user.Name); name != "" {
		if first, _, ok := strings.Cut(name, " "); ok && first != "" {
			return first
		}
		return name
	}
	if at := strings.IndexByte(user.Email, '@'); at > 0 {
		return user.Email[:at]
	}
	return "there"
}

func (h *AuthHandler) issueTokensAndRespond(w http.ResponseWriter, status int, user model.User, orgID, role string) {
	accessToken, err := auth.IssueAccessToken(h.privateKey, h.issuer, h.audience, user.ID.String(), orgID, role, h.accessTTL)
	if err != nil {
		slog.Error("failed to issue access token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	refreshToken, err := auth.IssueRefreshToken(h.signingKey, user.ID.String(), h.refreshTTL)
	if err != nil {
		slog.Error("failed to issue refresh token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Store refresh token hash for revocation tracking.
	storedRefresh := model.RefreshToken{
		UserID:    user.ID,
		TokenHash: hashToken(refreshToken),
		ExpiresAt: time.Now().Add(h.refreshTTL),
	}
	if err := h.db.Create(&storedRefresh).Error; err != nil {
		slog.Error("failed to store refresh token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Get org memberships for the response.
	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", user.ID).Find(&memberships)

	orgs := make([]orgMemberDTO, 0, len(memberships))
	for _, m := range memberships {
		orgs = append(orgs, orgMemberDTO{
			ID:   m.OrgID.String(),
			Name: m.Org.Name,
			Role: m.Role,
			BYOK: m.Org.BYOK,
		})
	}

	writeJSON(w, status, authResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(h.accessTTL.Seconds()),
		User: userResponse{
			ID:             user.ID.String(),
			Email:          user.Email,
			Name:           user.Name,
			EmailConfirmed: user.EmailConfirmedAt != nil,
		},
		Orgs: orgs,
	})
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
