package handler

import (
	"context"
	"crypto/rsa"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/goroutine"
)

type loginAttempt struct {
	failures int
	firstAt  time.Time
}

type AuthHandler struct {
	db          *gorm.DB
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

	loginMu       sync.Mutex
	loginAttempts map[string]*loginAttempt // keyed by email
}

func NewAuthHandler(db *gorm.DB, privateKey *rsa.PrivateKey, signingKey []byte, issuer, audience string, accessTTL, refreshTTL time.Duration, emailSender email.Sender, frontendURL string, autoConfirmEmail bool) *AuthHandler {
	h := &AuthHandler{
		db:               db,
		privateKey:       privateKey,
		signingKey:       signingKey,
		issuer:           issuer,
		audience:         audience,
		accessTTL:        accessTTL,
		refreshTTL:       refreshTTL,
		emailSender:      emailSender,
		frontendURL:      frontendURL,
		autoConfirmEmail: autoConfirmEmail,
		loginAttempts:    make(map[string]*loginAttempt),
	}

	return h
}

// SetAdminMode restricts login to the given platform admin emails only.
// When enabled, non-admin users receive a 403 on login/register. Emails are
// stored lowercased so lookups are case-insensitive; use isPlatformAdminEmail
// at call sites to normalize the runtime value.
func (h *AuthHandler) SetAdminMode(emails []string) {
	h.adminMode = true
	h.platformAdminEmails = make(map[string]bool, len(emails))
	for _, e := range emails {
		normalized := normalizePlatformAdminEmail(e)
		if normalized != "" {
			h.platformAdminEmails[normalized] = true
		}
	}
}

// SetPlatformAdminEmails records which emails are platform admins so that
// /auth/me can return is_platform_admin without enabling admin-only login mode.
// Emails are stored lowercased for case-insensitive matching.
func (h *AuthHandler) SetPlatformAdminEmails(emails []string) {
	if h.platformAdminEmails == nil {
		h.platformAdminEmails = make(map[string]bool, len(emails))
	}
	for _, e := range emails {
		normalized := normalizePlatformAdminEmail(e)
		if normalized != "" {
			h.platformAdminEmails[normalized] = true
		}
	}
}

// normalizePlatformAdminEmail lowercases and trims an email so equality checks
// against the platform admin allowlist are case-insensitive.
func normalizePlatformAdminEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// isPlatformAdminEmail returns true when the given email (any case) is in the
// platform admin allowlist.
func (h *AuthHandler) isPlatformAdminEmail(email string) bool {
	if len(h.platformAdminEmails) == 0 {
		return false
	}
	return h.platformAdminEmails[normalizePlatformAdminEmail(email)]
}

// StartCleanup starts a background goroutine that evicts stale login attempts
// every 5 minutes. The goroutine stops when ctx is cancelled.
func (h *AuthHandler) StartCleanup(ctx context.Context) {
	goroutine.Go(func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.loginMu.Lock()
				cutoff := time.Now().Add(-15 * time.Minute)
				for email, a := range h.loginAttempts {
					if a.firstAt.Before(cutoff) {
						delete(h.loginAttempts, email)
					}
				}
				h.loginMu.Unlock()
			}
		}
	})
}
