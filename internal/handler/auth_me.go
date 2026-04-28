package handler

import (
	"net/http"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

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

	// Bulk-load full plan rows for every plan slug referenced by the user's
	// orgs in one query, then map slug -> plan. Avoids an N+1 over plans.
	plans := loadPlans(h.db, memberships)

	orgs := make([]orgMemberDTO, 0, len(memberships))
	for _, m := range memberships {
		dto := orgMemberDTO{
			ID:      m.OrgID.String(),
			Name:    m.Org.Name,
			Role:    m.Role,
			BYOK:    m.Org.BYOK,
			LogoURL: m.Org.LogoURL,
			Plan:    plans[m.Org.PlanSlug],
		}
		if m.Role == "owner" || m.Role == "admin" {
			balance, err := h.credits.Balance(m.OrgID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load balance"})
				return
			}
			dto.Credits = &balance
		}
		orgs = append(orgs, dto)
	}

	isPlatformAdmin := len(h.platformAdminEmails) > 0 && h.platformAdminEmails[user.Email]

	writeJSON(w, http.StatusOK, meResponse{
		User: userResponse{
			ID:             user.ID.String(),
			Email:          user.Email,
			Name:           user.Name,
			EmailConfirmed: user.EmailConfirmedAt != nil,
		},
		Orgs:            orgs,
		IsPlatformAdmin: isPlatformAdmin,
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

