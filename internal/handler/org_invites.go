package handler

import (
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/usehiveloop/hiveloop/internal/email"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

type OrgInviteHandler struct {
	db          *gorm.DB
	emailSender email.Sender
	frontendURL string
	inviteTTL   time.Duration
}

func NewOrgInviteHandler(db *gorm.DB, sender email.Sender, frontendURL string) *OrgInviteHandler {
	return &OrgInviteHandler{
		db:          db,
		emailSender: sender,
		frontendURL: strings.TrimRight(frontendURL, "/"),
		inviteTTL:   7 * 24 * time.Hour,
	}
}

type createOrgInviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type orgInviteResponse struct {
	ID             string  `json:"id"`
	OrgID          string  `json:"org_id"`
	Email          string  `json:"email"`
	Role           string  `json:"role"`
	InvitedByID    string  `json:"invited_by_id"`
	InvitedByName  string  `json:"invited_by_name,omitempty"`
	InvitedByEmail string  `json:"invited_by_email,omitempty"`
	ExpiresAt      string  `json:"expires_at"`
	CreatedAt      string  `json:"created_at"`
	AcceptedAt     *string `json:"accepted_at,omitempty"`
	RevokedAt      *string `json:"revoked_at,omitempty"`
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

func invitedDisplayName(u *model.User) string {
	return inviterDisplayName(u)
}

func isValidInviteRole(role string) bool {
	return role == "admin" || role == "member" || role == "viewer"
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
