package handler

import (
	"net/http"


	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)
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
