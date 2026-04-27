package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"


	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/model"
)

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

	if user.PasswordHash == "" || !auth.CheckPassword(user.PasswordHash, req.Password) {
		h.recordLoginFailure(req.Email)
		slog.Warn("login failed", "email", req.Email, "reason", "invalid_credentials")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	h.clearLoginFailures(req.Email)

	// Admin mode: reject non-admin users
	if h.adminMode && !h.platformAdminEmails[user.Email] {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
		return
	}

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

