package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/email"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// OrgInviteHandler implements the org member invitation endpoints.
type OrgInviteHandler struct {
	db          *gorm.DB
	emailSender email.Sender
	frontendURL string
	inviteTTL   time.Duration
}

// NewOrgInviteHandler builds a new OrgInviteHandler. The frontendURL is used to build
// the invite landing-page link delivered to invitees via email.
func NewOrgInviteHandler(db *gorm.DB, sender email.Sender, frontendURL string) *OrgInviteHandler {
	return &OrgInviteHandler{
		db:          db,
		emailSender: sender,
		frontendURL: strings.TrimRight(frontendURL, "/"),
		inviteTTL:   7 * 24 * time.Hour,
	}
}

// --- DTOs ---

type createOrgInviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type orgInviteResponse struct {
	ID              string  `json:"id"`
	OrgID           string  `json:"org_id"`
	Email           string  `json:"email"`
	Role            string  `json:"role"`
	InvitedByID     string  `json:"invited_by_id"`
	InvitedByName   string  `json:"invited_by_name,omitempty"`
	InvitedByEmail  string  `json:"invited_by_email,omitempty"`
	ExpiresAt       string  `json:"expires_at"`
	CreatedAt       string  `json:"created_at"`
	AcceptedAt      *string `json:"accepted_at,omitempty"`
	RevokedAt       *string `json:"revoked_at,omitempty"`
}

type orgInvitePreviewResponse struct {
	OrgID       string `json:"org_id"`
	OrgName     string `json:"org_name"`
	InviterName string `json:"inviter_name"`
	Role        string `json:"role"`
	Email       string `json:"email"`
	ExpiresAt   string `json:"expires_at"`
}

type orgMemberResponse struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	JoinedAt string `json:"joined_at"`
}

type orgInviteAcceptResponse struct {
	OrgID   string `json:"org_id"`
	OrgName string `json:"org_name"`
	Role    string `json:"role"`
}

type listOrgInvitesResponse struct {
	Data []orgInviteResponse `json:"data"`
}

type listOrgMembersResponse struct {
	Data []orgMemberResponse `json:"data"`
}

// --- Helpers ---

func toInviteResponse(inv model.OrgInvite) orgInviteResponse {
	resp := orgInviteResponse{
		ID:             inv.ID.String(),
		OrgID:          inv.OrgID.String(),
		Email:          inv.Email,
		Role:           inv.Role,
		InvitedByID:    inv.InvitedByID.String(),
		InvitedByName:  inv.InvitedBy.Name,
		InvitedByEmail: inv.InvitedBy.Email,
		ExpiresAt:      inv.ExpiresAt.Format(time.RFC3339),
		CreatedAt:      inv.CreatedAt.Format(time.RFC3339),
	}
	if inv.AcceptedAt != nil {
		s := inv.AcceptedAt.Format(time.RFC3339)
		resp.AcceptedAt = &s
	}
	if inv.RevokedAt != nil {
		s := inv.RevokedAt.Format(time.RFC3339)
		resp.RevokedAt = &s
	}
	return resp
}

// inviterDisplayName uses user.Name if non-empty, else the local-part of email.
func inviterDisplayName(u *model.User) string {
	if u == nil {
		return "Someone"
	}
	if name := strings.TrimSpace(u.Name); name != "" {
		return name
	}
	if at := strings.IndexByte(u.Email, '@'); at > 0 {
		return u.Email[:at]
	}
	return "Someone"
}

// invitedDisplayName returns a display name for the user accepting an invite.
func invitedDisplayName(u *model.User) string {
	return inviterDisplayName(u)
}

func isValidInviteRole(role string) bool {
	return role == "admin" || role == "viewer"
}

func normalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func isValidEmail(addr string) bool {
	if addr == "" {
		return false
	}
	_, err := mail.ParseAddress(addr)
	return err == nil
}

// --- Handlers ---

// Create handles POST /v1/orgs/current/invites.
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be 'admin' or 'viewer'"})
		return
	}

	// If a user with this email already has a membership in the org → 409.
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

	// Existing pending (not accepted/revoked, not expired) invite → 409.
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

	// Load inviter to populate the email template.
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

// List handles GET /v1/orgs/current/invites.
// @Summary List pending invitations
// @Description Returns non-expired, non-accepted, non-revoked invites for the current org. Admin-only.
// @Tags org-invites
// @Produce json
// @Success 200 {object} listOrgInvitesResponse
// @Failure 403 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/invites [get]
func (h *OrgInviteHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing organization context"})
		return
	}

	var invites []model.OrgInvite
	if err := h.db.Preload("InvitedBy").
		Where("org_id = ? AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > ?", org.ID, time.Now()).
		Order("created_at DESC").
		Find(&invites).Error; err != nil {
		slog.Error("list invites", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list invites"})
		return
	}

	out := make([]orgInviteResponse, 0, len(invites))
	for _, inv := range invites {
		out = append(out, toInviteResponse(inv))
	}
	writeJSON(w, http.StatusOK, listOrgInvitesResponse{Data: out})
}

// Revoke handles DELETE /v1/orgs/current/invites/{id}.
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

// Resend handles POST /v1/orgs/current/invites/{id}/resend.
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

// ListMembers handles GET /v1/orgs/current/members.
// @Summary List members of the current organization
// @Description Returns every user with an active membership in the current org. Any member may call this.
// @Tags org-invites
// @Produce json
// @Success 200 {object} listOrgMembersResponse
// @Failure 403 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/members [get]
func (h *OrgInviteHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing organization context"})
		return
	}

	var memberships []model.OrgMembership
	if err := h.db.Preload("User").
		Where("org_id = ?", org.ID).
		Order("created_at ASC").
		Find(&memberships).Error; err != nil {
		slog.Error("list members", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list members"})
		return
	}

	out := make([]orgMemberResponse, 0, len(memberships))
	for _, m := range memberships {
		out = append(out, orgMemberResponse{
			UserID:   m.UserID.String(),
			Email:    m.User.Email,
			Name:     m.User.Name,
			Role:     m.Role,
			JoinedAt: m.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, listOrgMembersResponse{Data: out})
}

// Preview handles GET /v1/invites/{token} (public, no auth).
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
		OrgID:       invite.OrgID.String(),
		OrgName:     invite.Org.Name,
		InviterName: inviterDisplayName(&invite.InvitedBy),
		Role:        invite.Role,
		Email:       invite.Email,
		ExpiresAt:   invite.ExpiresAt.Format(time.RFC3339),
	})
}

// Accept handles POST /v1/invites/{token}/accept (JWT auth required).
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
	err = h.db.Transaction(func(tx *gorm.DB) error {
		membership := model.OrgMembership{
			UserID: user.ID,
			OrgID:  invite.OrgID,
			Role:   invite.Role,
		}
		if err := tx.Create(&membership).Error; err != nil {
			if !isDuplicateKeyError(err) {
				return fmt.Errorf("create membership: %w", err)
			}
		}
		if err := tx.Model(&model.OrgInvite{}).
			Where("id = ?", invite.ID).
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

	// Notify the admin who sent the invite.
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

// Decline handles POST /v1/invites/{token}/decline (JWT auth required).
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

// findValidInviteByToken looks up an invite by plaintext token, requiring it to be
// non-accepted, non-revoked and non-expired. Preloads Org and InvitedBy.
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
