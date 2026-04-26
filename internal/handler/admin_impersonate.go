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

// Impersonate issues tokens for the target user, allowing a platform admin
// to view the application as that user.
//
// @Summary Impersonate a user
// @Description Issues access and refresh tokens for the target user. Requires platform admin privileges.
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

	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", targetUser.ID).Find(&memberships)

	if len(memberships) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user has no organization memberships"})
		return
	}

	orgID := memberships[0].OrgID.String()
	role := memberships[0].Role

	accessToken, err := auth.IssueAccessToken(h.privateKey, h.issuer, h.audience, targetUser.ID.String(), orgID, role, h.accessTTL)
	if err != nil {
		slog.Error("impersonate: failed to issue access token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	refreshToken, err := auth.IssueRefreshToken(h.signingKey, targetUser.ID.String(), h.refreshTTL)
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

	admin, _ := middleware.UserFromContext(r.Context())
	adminEmail := ""
	if admin != nil {
		adminEmail = admin.Email
	}
	middleware.SetAdminAuditChanges(r, middleware.AdminAuditChanges{
		"action":        map[string]any{"old": nil, "new": "impersonate"},
		"target_user_id":    map[string]any{"old": nil, "new": targetUser.ID.String()},
		"target_email":      map[string]any{"old": nil, "new": targetUser.Email},
		"impersonator_email": map[string]any{"old": nil, "new": adminEmail},
	})

	orgs := make([]orgMemberDTO, 0, len(memberships))
	for _, membership := range memberships {
		orgs = append(orgs, orgMemberDTO{
			ID:   membership.OrgID.String(),
			Name: membership.Org.Name,
			Role: membership.Role,
			BYOK: membership.Org.BYOK,
		})
	}

	slog.Info("admin impersonating user", "admin_email", adminEmail, "target_user_id", targetUser.ID, "target_email", targetUser.Email)

	writeJSON(w, http.StatusOK, authResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(h.accessTTL.Seconds()),
		User: userResponse{
			ID:             targetUser.ID.String(),
			Email:          targetUser.Email,
			Name:           targetUser.Name,
			EmailConfirmed: targetUser.EmailConfirmedAt != nil,
		},
		Orgs: orgs,
	})
}