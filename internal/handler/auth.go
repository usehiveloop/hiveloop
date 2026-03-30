package handler

import (
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/llmvault/llmvault/internal/auth"
	"github.com/llmvault/llmvault/internal/email"
	"github.com/llmvault/llmvault/internal/middleware"
	"github.com/llmvault/llmvault/internal/model"
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

	// Cleanup stale login attempts every 5 minutes.
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			h.loginMu.Lock()
			cutoff := time.Now().Add(-15 * time.Minute)
			for email, a := range h.loginAttempts {
				if a.firstAt.Before(cutoff) {
					delete(h.loginAttempts, email)
				}
			}
			h.loginMu.Unlock()
		}
	}()

	return h
}

// --- Request / Response types ---

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	OrgID    string `json:"org_id,omitempty"` // optional: scope token to a specific org
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
	OrgID        string `json:"org_id,omitempty"` // optional: switch org
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type authResponse struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	ExpiresIn    int            `json:"expires_in"` // seconds
	User         userResponse   `json:"user"`
	Orgs         []orgMemberDTO `json:"orgs"`
}

type userResponse struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	EmailConfirmed bool   `json:"email_confirmed"`
}

type orgMemberDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type meResponse struct {
	User userResponse   `json:"user"`
	Orgs []orgMemberDTO `json:"orgs"`
}

type statusResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// --- Handlers ---

// Register handles POST /auth/register.
// @Summary Register a new user
// @Description Creates a new user account, organization, and sends a confirmation email.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body registerRequest true "Registration parameters"
// @Success 201 {object} authResponse
// @Failure 400 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Router /auth/register [post]
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Name = strings.TrimSpace(req.Name)

	if req.Email == "" || req.Password == "" || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email, password, and name are required"})
		return
	}
	if len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 8 characters"})
		return
	}

	// Check if email is taken.
	var existing model.User
	if err := h.db.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
		return
	}

	// Hash password.
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		slog.Error("failed to hash password", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Create user, org, and membership in a transaction.
	var user model.User
	var org model.Org
	var membership model.OrgMembership

	err = h.db.Transaction(func(tx *gorm.DB) error {
		user = model.User{
			Email:        req.Email,
			PasswordHash: hash,
			Name:         req.Name,
		}
		if err := tx.Create(&user).Error; err != nil {
			return fmt.Errorf("creating user: %w", err)
		}

		org = model.Org{
			Name: fmt.Sprintf("%s's Workspace", req.Name),
		}
		if err := tx.Create(&org).Error; err != nil {
			return fmt.Errorf("creating org: %w", err)
		}

		membership = model.OrgMembership{
			UserID: user.ID,
			OrgID:  org.ID,
			Role:   "admin",
		}
		if err := tx.Create(&membership).Error; err != nil {
			return fmt.Errorf("creating membership: %w", err)
		}

		return nil
	})
	if err != nil {
		slog.Error("failed to register user", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create account"})
		return
	}

	if h.autoConfirmEmail {
		// Auto-confirm: mark email as confirmed immediately.
		now := time.Now()
		h.db.Model(&user).Update("email_confirmed_at", &now)
	} else {
		// Generate and store email verification token.
		plainToken, tokenHash, err := model.GenerateVerificationToken()
		if err != nil {
			slog.Error("failed to generate verification token", "error", err)
		} else {
			verification := model.EmailVerification{
				UserID:    user.ID,
				TokenHash: tokenHash,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}
			if err := h.db.Create(&verification).Error; err != nil {
				slog.Error("failed to store verification token", "error", err)
			} else {
				confirmURL := fmt.Sprintf("%s/auth/confirm-email?token=%s", h.frontendURL, plainToken)
				_ = h.emailSender.Send(r.Context(), email.Message{
					To:      req.Email,
					Subject: "Confirm your email",
					Body:    confirmURL,
				})
			}
		}
	}

	slog.Info("user registered", "user_id", user.ID, "email", user.Email)
	h.issueTokensAndRespond(w, http.StatusCreated, user, org.ID.String(), "admin")
}

// Login handles POST /auth/login.
// @Summary Log in
// @Description Authenticates a user with email and password.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body loginRequest true "Login parameters"
// @Success 200 {object} authResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Router /auth/login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	if req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password are required"})
		return
	}

	// Per-account rate limiting: 5 failed attempts per 15 minutes.
	if h.isLoginLocked(req.Email) {
		slog.Warn("login rate limited", "email", req.Email)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		h.recordLoginFailure(req.Email)
		slog.Warn("login failed", "email", req.Email, "reason", "invalid_credentials")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		h.recordLoginFailure(req.Email)
		slog.Warn("login failed", "email", req.Email, "reason", "invalid_credentials")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	h.clearLoginFailures(req.Email)

	// Get user's memberships.
	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", user.ID).Find(&memberships)

	if len(memberships) == 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization memberships"})
		return
	}

	// Determine which org to scope the token to.
	orgID := memberships[0].OrgID.String()
	role := memberships[0].Role
	if req.OrgID != "" {
		found := false
		for _, m := range memberships {
			if m.OrgID.String() == req.OrgID {
				orgID = req.OrgID
				role = m.Role
				found = true
				break
			}
		}
		if !found {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of the requested organization"})
			return
		}
	}

	slog.Info("user logged in", "user_id", user.ID, "email", user.Email)
	h.issueTokensAndRespond(w, http.StatusOK, user, orgID, role)
}

// Refresh handles POST /auth/refresh.
// @Summary Refresh tokens
// @Description Exchanges a refresh token for new access and refresh tokens.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body refreshRequest true "Refresh parameters"
// @Success 200 {object} authResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Router /auth/refresh [post]
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.RefreshToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "refresh_token is required"})
		return
	}

	// Validate the refresh JWT.
	userID, _, err := auth.ValidateRefreshToken(h.signingKey, req.RefreshToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
		return
	}

	// Check the token hash in the database (revocation check + rotation).
	tokenHash := hashToken(req.RefreshToken)
	var storedToken model.RefreshToken
	if err := h.db.Where("token_hash = ? AND revoked_at IS NULL", tokenHash).First(&storedToken).Error; err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "refresh token revoked or not found"})
		return
	}

	if time.Now().After(storedToken.ExpiresAt) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "refresh token expired"})
		return
	}

	// Revoke the old refresh token (rotation).
	now := time.Now()
	h.db.Model(&storedToken).Update("revoked_at", &now)

	// Get memberships to determine org/role.
	var user model.User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}

	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", user.ID).Find(&memberships)

	if len(memberships) == 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization memberships"})
		return
	}

	orgID := memberships[0].OrgID.String()
	role := memberships[0].Role
	if req.OrgID != "" {
		found := false
		for _, m := range memberships {
			if m.OrgID.String() == req.OrgID {
				orgID = req.OrgID
				role = m.Role
				found = true
				break
			}
		}
		if !found {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of the requested organization"})
			return
		}
	}

	h.issueTokensAndRespond(w, http.StatusOK, user, orgID, role)
}

// Logout handles POST /auth/logout.
// @Summary Log out
// @Description Revokes a refresh token.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body logoutRequest true "Logout parameters"
// @Success 200 {object} statusResponse
// @Security BearerAuth
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.RefreshToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "refresh_token is required"})
		return
	}

	tokenHash := hashToken(req.RefreshToken)
	now := time.Now()
	h.db.Model(&model.RefreshToken{}).Where("token_hash = ? AND revoked_at IS NULL", tokenHash).Update("revoked_at", &now)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Me handles GET /auth/me.
// @Summary Get current user
// @Description Returns the current user and their organization memberships.
// @Tags auth
// @Produce json
// @Success 200 {object} meResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /auth/me [get]
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AuthClaimsFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var user model.User
	if err := h.db.Where("id = ?", claims.UserID).First(&user).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

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

	writeJSON(w, http.StatusOK, meResponse{
		User: userResponse{
			ID:             user.ID.String(),
			Email:          user.Email,
			Name:           user.Name,
			EmailConfirmed: user.EmailConfirmedAt != nil,
		},
		Orgs: orgs,
	})
}

// --- Email confirmation & password reset ---

type confirmEmailRequest struct {
	Token string `json:"token"`
}

type resendConfirmationRequest struct {
	Email string `json:"email"`
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ConfirmEmail handles POST /auth/confirm-email.
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
	_ = h.emailSender.Send(r.Context(), email.Message{
		To:      user.Email,
		Subject: "Confirm your email",
		Body:    confirmURL,
	})

	writeJSON(w, http.StatusOK, genericResponse)
}

// ForgotPassword handles POST /auth/forgot-password.
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

	// Look up user. Always return 200 regardless (anti-enumeration).
	var user model.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		writeJSON(w, http.StatusOK, genericResponse)
		return
	}

	// Rate limit: max 3 per user per 15 minutes.
	var count int64
	cutoff := time.Now().Add(-15 * time.Minute)
	h.db.Model(&model.PasswordReset{}).Where("user_id = ? AND created_at > ?", user.ID, cutoff).Count(&count)
	if count >= 3 {
		writeJSON(w, http.StatusOK, genericResponse)
		return
	}

	plainToken, tokenHash, err := model.GenerateResetToken()
	if err != nil {
		slog.Error("failed to generate reset token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	reset := model.PasswordReset{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	if err := h.db.Create(&reset).Error; err != nil {
		slog.Error("failed to store reset token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	resetURL := fmt.Sprintf("%s/auth/reset-password?token=%s", h.frontendURL, plainToken)
	_ = h.emailSender.Send(r.Context(), email.Message{
		To:      user.Email,
		Subject: "Reset your password",
		Body:    resetURL,
	})

	slog.Info("password reset requested", "email", user.Email)
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
		slog.Error("failed to hash password", "error", err)
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
		// Revoke all refresh tokens for this user (invalidate all sessions).
		if err := tx.Model(&model.RefreshToken{}).
			Where("user_id = ? AND revoked_at IS NULL", reset.UserID).
			Update("revoked_at", &now).Error; err != nil {
			return err
		}
		// Invalidate all other pending reset tokens for this user.
		if err := tx.Model(&model.PasswordReset{}).
			Where("user_id = ? AND used_at IS NULL AND id != ?", reset.UserID, reset.ID).
			Update("used_at", &now).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		slog.Error("failed to reset password", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	slog.Info("password reset completed", "user_id", reset.UserID)
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

	if !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "current password is incorrect"})
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		slog.Error("failed to hash password", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	now := time.Now()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&user).Update("password_hash", newHash).Error; err != nil {
			return err
		}
		// Revoke all refresh tokens (forces re-login on all devices).
		if err := tx.Model(&model.RefreshToken{}).
			Where("user_id = ? AND revoked_at IS NULL", user.ID).
			Update("revoked_at", &now).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		slog.Error("failed to change password", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	slog.Info("password changed", "user_id", user.ID)
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Password changed. Please log in again.",
	})
}

// --- Login rate limiting ---

const (
	maxLoginFailures   = 5
	loginLockoutWindow = 15 * time.Minute
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

// --- Helpers ---

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
