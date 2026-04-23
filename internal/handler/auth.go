package handler

import (
	"context"
	"crypto/rsa"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/email"
)

type AuthHandler struct {
	db          *gorm.DB
	redis       *redis.Client
	privateKey  *rsa.PrivateKey
	signingKey  []byte // HMAC key for refresh tokens (JWT_SIGNING_KEY)
	issuer      string
	audience    string
	accessTTL   time.Duration
	refreshTTL  time.Duration
	emailSender      email.Sender
	frontendURL      string
	autoConfirmEmail bool

	// Admin mode: when true, login is restricted to platform admin emails only.
	// Used by the admin panel deployment to prevent non-admin users from logging in.
	adminMode          bool
	platformAdminEmails map[string]bool
}

// NewAuthHandler constructs an AuthHandler. A non-nil redis client is required
// for login rate limiting; pass a connected client from bootstrap.Deps.Redis.
func NewAuthHandler(db *gorm.DB, redisClient *redis.Client, privateKey *rsa.PrivateKey, signingKey []byte, issuer, audience string, accessTTL, refreshTTL time.Duration, emailSender email.Sender, frontendURL string, autoConfirmEmail bool) *AuthHandler {
	h := &AuthHandler{
		db:               db,
		redis:            redisClient,
		privateKey:       privateKey,
		signingKey:       signingKey,
		issuer:           issuer,
		audience:         audience,
		accessTTL:        accessTTL,
		refreshTTL:       refreshTTL,
		emailSender:      emailSender,
		frontendURL:      frontendURL,
		autoConfirmEmail: autoConfirmEmail,
	}

	return h
}

// SetAdminMode restricts login to the given platform admin emails only.
// When enabled, non-admin users receive a 403 on login/register.
func (h *AuthHandler) SetAdminMode(emails []string) {
	h.adminMode = true
	h.platformAdminEmails = make(map[string]bool, len(emails))
	for _, e := range emails {
		trimmed := strings.TrimSpace(e)
		if trimmed != "" {
			h.platformAdminEmails[trimmed] = true
		}
	}
}

// SetPlatformAdminEmails records which emails are platform admins so that
// /auth/me can return is_platform_admin without enabling admin-only login mode.
func (h *AuthHandler) SetPlatformAdminEmails(emails []string) {
	if h.platformAdminEmails == nil {
		h.platformAdminEmails = make(map[string]bool, len(emails))
	}
	for _, e := range emails {
		trimmed := strings.TrimSpace(e)
		if trimmed != "" {
			h.platformAdminEmails[trimmed] = true
		}
	}
}

// StartCleanup is a no-op retained for API compatibility. Rate-limit state is
// now kept in Redis with TTLs so no periodic eviction is required.
func (h *AuthHandler) StartCleanup(ctx context.Context) {}
