package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// @Summary Revoke a pending invitation
// @Description Marks an invite as revoked. Admin-only. Already-accepted invites cannot be revoked.
// @Tags org-invites
// @Param id path string true "Invite ID"
// @Success 204
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/invites/{id} [delete]
func (h *OrgInviteHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing organization context"})
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid invite id"})
		return
	}

	var invite model.OrgInvite
	if err := h.db.Where("id = ? AND org_id = ?", id, org.ID).First(&invite).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite not found"})
			return
		}
		slog.Error("load invite for revoke", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if invite.AcceptedAt != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already accepted"})
		return
	}
	if invite.RevokedAt != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	now := time.Now()
	if err := h.db.Model(&invite).Update("revoked_at", &now).Error; err != nil {
		slog.Error("revoke invite", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke invite"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary Resend an invitation
// @Description Generates a new token and re-sends the invite email. Admin-only.
// @Tags org-invites
// @Produce json
// @Param id path string true "Invite ID"
// @Success 200 {object} orgInviteResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/invites/{id}/resend [post]
func (h *OrgInviteHandler) Resend(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing organization context"})
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid invite id"})
		return
	}

	var invite model.OrgInvite
	if err := h.db.Preload("InvitedBy").Where("id = ? AND org_id = ?", id, org.ID).First(&invite).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite not found"})
			return
		}
		slog.Error("load invite for resend", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if invite.AcceptedAt != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already accepted"})
		return
	}
	if invite.RevokedAt != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already revoked"})
		return
	}

	plaintext, tokenHash, err := model.GenerateInviteToken()
	if err != nil {
		slog.Error("generate invite token", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	newExpiry := time.Now().Add(h.inviteTTL)
	if err := h.db.Model(&invite).Updates(map[string]any{
		"token_hash": tokenHash,
		"expires_at": newExpiry,
	}).Error; err != nil {
		slog.Error("update invite on resend", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resend invite"})
		return
	}
	invite.TokenHash = tokenHash
	invite.ExpiresAt = newExpiry

	inviteURL := fmt.Sprintf("%s/invites/accept?token=%s", h.frontendURL, plaintext)
	if err := h.emailSender.SendTemplate(r.Context(), email.TemplateMessage{
		To:   invite.Email,
		Slug: email.TmplOrgInvite,
		Variables: email.TemplateVars{
			"firstName":   "there",
			"inviterName": inviterDisplayName(&invite.InvitedBy),
			"orgName":     org.Name,
			"role":        invite.Role,
			"inviteUrl":   inviteURL,
			"expiresIn":   "7 days",
		},
	}); err != nil {
		slog.Error("resend invite email", "error", err)
	}

	writeJSON(w, http.StatusOK, toInviteResponse(invite))
}
