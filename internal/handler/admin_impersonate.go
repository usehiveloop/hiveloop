package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// impersonationAccessTTL caps the lifetime of impersonation access tokens to
// reduce the window of abuse if a token leaks. Normal user sessions keep their
// configured TTL; this override applies only to /admin/v1/users/{id}/impersonate.
const impersonationAccessTTL = 30 * time.Minute

// Impersonate issues tokens for the target user, allowing a platform admin
// to view the application as that user.
//
// @Summary Impersonate a user
// @Description Issues short-lived access and refresh tokens tagged as an impersonation session. Requires platform admin privileges.
// @Tags admin
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} authResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /admin/v1/users/{id}/impersonate [post]
func (h *AdminHandler) Impersonate(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(targetID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}

	var targetUser model.User
	if err := h.db.Where("id = ?", targetID).First(&targetUser).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	if targetUser.BannedAt != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot impersonate a banned user"})
		return
	}

	if targetUser.EmailConfirmedAt == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot impersonate a user with unconfirmed email"})
		return
	}

	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", targetUser.ID).Find(&memberships)

	if len(memberships) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user has no organization memberships"})
		return
	}

	orgID := memberships[0].OrgID.String()
	role := memberships[0].Role

	admin, _ := middleware.UserFromContext(r.Context())
	adminEmail := ""
	adminID := ""
	if admin != nil {
		adminEmail = admin.Email
		adminID = admin.ID.String()
	}

	accessTTL := h.accessTTL
	if accessTTL > impersonationAccessTTL {
		accessTTL = impersonationAccessTTL
	}

	accessToken, err := auth.IssueAccessTokenWithImpersonation(h.privateKey, h.issuer, h.audience, targetUser.ID.String(), orgID, role, adminID, accessTTL)
	if err != nil {
		slog.Error("impersonate: failed to issue access token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	refreshToken, err := auth.IssueRefreshTokenWithImpersonation(h.signingKey, targetUser.ID.String(), adminID, h.refreshTTL)
	if err != nil {
		slog.Error("impersonate: failed to issue refresh token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	storedRefresh := model.RefreshToken{
		UserID:    targetUser.ID,
		TokenHash: hashToken(refreshToken),
		ExpiresAt: time.Now().Add(h.refreshTTL),
	}
	if err := h.db.Create(&storedRefresh).Error; err != nil {
		slog.Error("impersonate: failed to store refresh token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	middleware.SetAdminAuditChanges(r, middleware.AdminAuditChanges{
		"action":             map[string]any{"old": nil, "new": "impersonate"},
		"target_user_id":     map[string]any{"old": nil, "new": targetUser.ID.String()},
		"target_email":       map[string]any{"old": nil, "new": targetUser.Email},
		"impersonator_id":    map[string]any{"old": nil, "new": adminID},
		"impersonator_email": map[string]any{"old": nil, "new": adminEmail},
		"access_ttl_seconds": map[string]any{"old": nil, "new": int(accessTTL.Seconds())},
	})

	orgs := make([]orgMemberDTO, 0, len(memberships))
	for _, membership := range memberships {
		orgs = append(orgs, orgMemberDTO{
			ID:   membership.OrgID.String(),
			Name: membership.Org.Name,
			Role: membership.Role,
		})
	}

	slog.Info("admin impersonation token issued",
		"admin_id", adminID,
		"admin_email", adminEmail,
		"target_user_id", targetUser.ID,
		"target_email", targetUser.Email,
		"access_ttl_seconds", int(accessTTL.Seconds()),
	)

	// Use a minimal user payload here: the standard userResponse includes an
	// email_confirmed bool that always serializes (no omitempty on primitive),
	// which would leak confirmation status. Since impersonation now requires
	// confirmed email, there is no reason to echo it back.
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":     accessToken,
		"refresh_token":    refreshToken,
		"expires_in":       int(accessTTL.Seconds()),
		"impersonation":    true,
		"impersonation_of": adminID,
		"user": map[string]any{
			"id":    targetUser.ID.String(),
			"email": targetUser.Email,
			"name":  targetUser.Name,
		},
		"orgs": orgs,
	})
}