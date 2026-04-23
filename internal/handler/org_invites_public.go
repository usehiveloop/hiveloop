package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// @Summary Preview an invitation (public)
// @Description Returns basic invite details by plaintext token. Returns 404 for invalid/expired/used/revoked tokens without distinguishing.
// @Tags org-invites
// @Produce json
// @Param token path string true "Invite token (plaintext)"
// @Success 200 {object} orgInvitePreviewResponse
// @Failure 404 {object} errorResponse
// @Router /v1/invites/{token} [get]
func (h *OrgInviteHandler) Preview(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid or expired invite"})
		return
	}

	invite, ok := h.findValidInviteByToken(token)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid or expired invite"})
		return
	}

	writeJSON(w, http.StatusOK, orgInvitePreviewResponse{
		OrgName:     invite.Org.Name,
		InviterName: inviterDisplayName(&invite.InvitedBy),
		Role:        invite.Role,
		ExpiresAt:   invite.ExpiresAt.Format(time.RFC3339),
	})
}

// @Summary Accept an invitation
// @Description Accepts an invite and creates the corresponding org membership. The authenticated user's email must match the invite email.
// @Tags org-invites
// @Produce json
// @Param token path string true "Invite token (plaintext)"
// @Success 200 {object} orgInviteAcceptResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/invites/{token}/accept [post]
func (h *OrgInviteHandler) Accept(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AuthClaimsFromContext(r.Context())
	if !ok || claims == nil || claims.UserID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid user context"})
		return
	}

	token := chi.URLParam(r, "token")

	var user model.User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}

	var (
		invite        model.OrgInvite
		inviteFound   bool
		emailMismatch bool
		alreadyMember bool
	)

	now := time.Now()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		// Lock the invite row for the duration of the transaction so two
		// concurrent accept requests can't both pass the validity check.
		locked, ok := h.findValidInviteByTokenForUpdate(tx, token)
		if !ok {
			return nil
		}
		invite = locked
		inviteFound = true

		if normalizeEmail(user.Email) != normalizeEmail(invite.Email) {
			emailMismatch = true
			return nil
		}

		// Check whether the user is already a member of the target org. If
		// they are, mark the invite accepted (idempotent) and skip creating a
		// second membership to avoid relying on the unique constraint for
		// de-duplication.
		var existingCount int64
		if err := tx.Model(&model.OrgMembership{}).
			Where("user_id = ? AND org_id = ?", user.ID, invite.OrgID).
			Count(&existingCount).Error; err != nil {
			return fmt.Errorf("check existing membership: %w", err)
		}

		if existingCount == 0 {
			membership := model.OrgMembership{
				UserID: user.ID,
				OrgID:  invite.OrgID,
				Role:   invite.Role,
			}
			if err := tx.Create(&membership).Error; err != nil {
				if isDuplicateKeyError(err) {
					// Another transaction created the membership between our
					// count and insert; treat as already-a-member.
					alreadyMember = true
				} else {
					return fmt.Errorf("create membership: %w", err)
				}
			}
		} else {
			alreadyMember = true
		}

		if err := tx.Model(&model.OrgInvite{}).
			Where("id = ? AND accepted_at IS NULL", invite.ID).
			Update("accepted_at", &now).Error; err != nil {
			return fmt.Errorf("mark invite accepted: %w", err)
		}
		return nil
	})
	if err != nil {
		slog.Error("accept invite", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to accept invite"})
		return
	}
	if !inviteFound {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid or expired invite"})
		return
	}
	if emailMismatch {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": fmt.Sprintf("This invite was sent to %s. Sign in with that account to accept.", invite.Email),
		})
		return
	}
	if alreadyMember {
		writeJSON(w, http.StatusOK, orgInviteAcceptResponse{
			OrgID:   invite.OrgID.String(),
			OrgName: invite.Org.Name,
			Role:    invite.Role,
		})
		return
	}

	if h.emailSender != nil {
		if err := h.emailSender.SendTemplate(r.Context(), email.TemplateMessage{
			To:   invite.InvitedBy.Email,
			Slug: email.TmplOrgInviteAccepted,
			Variables: email.TemplateVars{
				"adminFirstName": inviterDisplayName(&invite.InvitedBy),
				"invitedName":    invitedDisplayName(&user),
				"invitedEmail":   user.Email,
				"orgName":        invite.Org.Name,
				"role":           invite.Role,
			},
		}); err != nil {
			slog.Error("send invite-accepted email", "error", err)
		}
	}

	writeJSON(w, http.StatusOK, orgInviteAcceptResponse{
		OrgID:   invite.OrgID.String(),
		OrgName: invite.Org.Name,
		Role:    invite.Role,
	})
}

// @Summary Decline an invitation
// @Description Declines an invite and marks it as terminally revoked.
// @Tags org-invites
// @Param token path string true "Invite token (plaintext)"
// @Success 204
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/invites/{token}/decline [post]
func (h *OrgInviteHandler) Decline(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AuthClaimsFromContext(r.Context())
	if !ok || claims == nil || claims.UserID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid user context"})
		return
	}

	token := chi.URLParam(r, "token")
	invite, ok := h.findValidInviteByToken(token)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid or expired invite"})
		return
	}

	var user model.User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}
	if normalizeEmail(user.Email) != normalizeEmail(invite.Email) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": fmt.Sprintf("This invite was sent to %s. Sign in with that account to accept.", invite.Email),
		})
		return
	}

	now := time.Now()
	if err := h.db.Model(&model.OrgInvite{}).Where("id = ?", invite.ID).Update("revoked_at", &now).Error; err != nil {
		slog.Error("decline invite", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decline invite"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *OrgInviteHandler) findValidInviteByToken(token string) (model.OrgInvite, bool) {
	if token == "" {
		return model.OrgInvite{}, false
	}
	hash := model.HashInviteToken(token)
	var invite model.OrgInvite
	err := h.db.Preload("Org").Preload("InvitedBy").
		Where("token_hash = ? AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?",
			hash, time.Now()).
		First(&invite).Error
	if err != nil {
		return model.OrgInvite{}, false
	}
	return invite, true
}

// findValidInviteByTokenForUpdate loads a valid invite within the given
// transaction and acquires a row-level lock (SELECT ... FOR UPDATE) so
// concurrent accept requests for the same token serialize.
func (h *OrgInviteHandler) findValidInviteByTokenForUpdate(tx *gorm.DB, token string) (model.OrgInvite, bool) {
	if token == "" {
		return model.OrgInvite{}, false
	}
	hash := model.HashInviteToken(token)
	var invite model.OrgInvite
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("token_hash = ? AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?",
			hash, time.Now()).
		First(&invite).Error
	if err != nil {
		return model.OrgInvite{}, false
	}
	// Preload associations after locking the base row. We use a fresh query
	// so the lock clause doesn't apply to the joined tables.
	if err := tx.Preload("Org").Preload("InvitedBy").
		Where("id = ?", invite.ID).First(&invite).Error; err != nil {
		return model.OrgInvite{}, false
	}
	return invite, true
}
