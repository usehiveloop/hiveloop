package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/model"
)
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

	// Admin mode: reject non-admin users
	if h.adminMode && !h.isPlatformAdminEmail(req.Email) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
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
			Role:   "owner",
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
				_ = h.emailSender.SendTemplate(r.Context(), email.TemplateMessage{
					To:   req.Email,
					Slug: email.TmplAuthConfirmEmail,
					Variables: email.TemplateVars{
						"firstName":       firstNameFrom(user),
						"email":           req.Email,
						"confirmationUrl": confirmURL,
						"expiresIn":       "24 hours",
					},
				})
			}
		}
	}

	slog.Info("user registered", "user_id", user.ID, "email", user.Email)
	h.issueTokensAndRespond(w, http.StatusCreated, user, org.ID.String(), "owner")
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
