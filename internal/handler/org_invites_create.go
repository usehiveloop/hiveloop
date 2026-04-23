package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// @Summary Invite a user to the current organization
// @Description Creates a pending invitation for the given email and role. Admin-only.
// @Tags org-invites
// @Accept json
// @Produce json
// @Param body body createOrgInviteRequest true "Invite parameters"
// @Success 201 {object} orgInviteResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/invites [post]
func (h *OrgInviteHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing organization context"})
		return
	}
	claims, ok := middleware.AuthClaimsFromContext(r.Context())
	if !ok || claims == nil || claims.UserID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	inviterID, err := uuid.Parse(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid user context"})
		return
	}

	var req createOrgInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	emailAddr := normalizeEmail(req.Email)
	role := strings.TrimSpace(req.Role)

	if !isValidEmail(emailAddr) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email"})
		return
	}
	if !isValidInviteRole(role) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be 'admin', 'member', or 'viewer'"})
		return
	}

	var existingUser model.User
	if err := h.db.Where("LOWER(email) = ?", emailAddr).First(&existingUser).Error; err == nil {
		var count int64
		if err := h.db.Model(&model.OrgMembership{}).
			Where("user_id = ? AND org_id = ?", existingUser.ID, org.ID).
			Count(&count).Error; err == nil && count > 0 {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already a member"})
			return
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Error("lookup user for invite", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	var existingInvite model.OrgInvite
	err = h.db.Where("org_id = ? AND email = ? AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?",
		org.ID, emailAddr, time.Now()).
		First(&existingInvite).Error
	if err == nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error":     "invite already pending",
			"invite_id": existingInvite.ID.String(),
		})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Error("lookup existing invite", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	plaintext, tokenHash, err := model.GenerateInviteToken()
	if err != nil {
		slog.Error("generate invite token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	invite := model.OrgInvite{
		OrgID:       org.ID,
		Email:       emailAddr,
		Role:        role,
		TokenHash:   tokenHash,
		InvitedByID: inviterID,
		ExpiresAt:   time.Now().Add(h.inviteTTL),
	}
	if err := h.db.Create(&invite).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "invite already pending"})
			return
		}
		slog.Error("create invite", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create invite"})
		return
	}

	var inviter model.User
	_ = h.db.Where("id = ?", inviterID).First(&inviter).Error
	invite.InvitedBy = inviter

	inviteURL := fmt.Sprintf("%s/invites/accept?token=%s", h.frontendURL, plaintext)
	if err := h.emailSender.SendTemplate(r.Context(), email.TemplateMessage{
		To:   emailAddr,
		Slug: email.TmplOrgInvite,
		Variables: email.TemplateVars{
			"firstName":   "there",
			"inviterName": inviterDisplayName(&inviter),
			"orgName":     org.Name,
			"role":        role,
			"inviteUrl":   inviteURL,
			"expiresIn":   "7 days",
		},
	}); err != nil {
		slog.Error("send invite email", "error", err)
	}

	writeJSON(w, http.StatusCreated, toInviteResponse(invite))
}
