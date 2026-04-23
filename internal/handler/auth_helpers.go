package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// Login rate limiting is Redis-backed with a composite (ip, email) scope.
// Two independent counters are maintained, each using INCR + EXPIRE with
// loginLockoutWindow TTL:
//
//   login:fail:ip:<ip>       primary bucket — defeats credential stuffing from
//                            a single source against many emails.
//   login:fail:email:<email> secondary bucket — keeps per-account lockout.
//
// Either bucket exceeding its threshold triggers a deny. On success both
// buckets are cleared. If Redis is unavailable we fail open so a cache
// outage never locks every user out.

const (
	loginFailIPPrefix    = "login:fail:ip:"
	loginFailEmailPrefix = "login:fail:email:"
	// maxLoginFailuresPerIP is deliberately higher than the per-email limit
	// so legitimate shared-NAT traffic is not disrupted by one careless user,
	// while still breaking credential stuffing that cycles many emails.
	maxLoginFailuresPerIP = 20
)

// clientIP returns the IP (host portion) of the request, falling back to
// RemoteAddr if it is not in host:port form.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if comma := strings.IndexByte(xff, ','); comma > 0 {
			return strings.TrimSpace(xff[:comma])
		}
		return strings.TrimSpace(xff)
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		return strings.TrimSpace(real)
	}
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}
	return r.RemoteAddr
}

func (h *AuthHandler) isLoginLocked(ctx context.Context, ip, email string) bool {
	if h.redis == nil {
		return false
	}

	if ip != "" {
		ipKey := loginFailIPPrefix + ip
		if n, err := h.redis.Get(ctx, ipKey).Int64(); err == nil && n >= maxLoginFailuresPerIP {
			return true
		}
	}
	if email != "" {
		emailKey := loginFailEmailPrefix + email
		if n, err := h.redis.Get(ctx, emailKey).Int64(); err == nil && n >= maxLoginFailures {
			return true
		}
	}
	return false
}

func (h *AuthHandler) recordLoginFailure(ctx context.Context, ip, email string) {
	if h.redis == nil {
		return
	}

	incr := func(key string) {
		n, err := h.redis.Incr(ctx, key).Result()
		if err != nil {
			slog.Warn("login rate limit incr failed", "key", key, "error", err)
			return
		}
		if n == 1 {
			if err := h.redis.Expire(ctx, key, loginLockoutWindow).Err(); err != nil {
				slog.Warn("login rate limit expire failed", "key", key, "error", err)
			}
		}
	}

	if ip != "" {
		incr(loginFailIPPrefix + ip)
	}
	if email != "" {
		incr(loginFailEmailPrefix + email)
	}
}

func (h *AuthHandler) clearLoginFailures(ctx context.Context, ip, email string) {
	if h.redis == nil {
		return
	}
	keys := make([]string, 0, 2)
	if ip != "" {
		keys = append(keys, loginFailIPPrefix+ip)
	}
	if email != "" {
		keys = append(keys, loginFailEmailPrefix+email)
	}
	if len(keys) == 0 {
		return
	}
	if err := h.redis.Del(ctx, keys...).Err(); err != nil {
		slog.Warn("login rate limit clear failed", "error", err)
	}
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
