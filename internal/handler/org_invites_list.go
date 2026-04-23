package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

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
