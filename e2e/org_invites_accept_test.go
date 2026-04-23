package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestOrgInvitePreviewAndAccept(t *testing.T) {
	ih := newInviteHarness(t)

	adminID, orgID := ih.createUserAndOrg(t, "admin", fmt.Sprintf("admin-%s@test.local", randomSuffix()), "admin")
	adminTok := ih.issueToken(t, adminID.String(), orgID.String(), "admin")

	inviteeEmail := fmt.Sprintf("invitee-%s@test.local", randomSuffix())
	inviteeID := ih.createUser(t, inviteeEmail, "Invitee")

	rr := ih.do(t, http.MethodPost, "/v1/orgs/current/invites",
		fmt.Sprintf(`{"email":%q,"role":"viewer"}`, inviteeEmail), adminTok)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create invite: got %d: %s", rr.Code, rr.Body.String())
	}
	var created inviteDTO
	_ = json.NewDecoder(rr.Body).Decode(&created)

	plaintext, hash, err := model.GenerateInviteToken()
	if err != nil {
		t.Fatalf("gen token: %v", err)
	}
	injected := model.OrgInvite{
		ID:          uuid.New(),
		OrgID:       orgID,
		Email:       inviteeEmail,
		Role:        "viewer",
		TokenHash:   hash,
		InvitedByID: adminID,
		ExpiresAt:   time.Now().Add(6 * 24 * time.Hour),
	}
	ih.db.Model(&model.OrgInvite{}).Where("id = ?", created.ID).Update("revoked_at", time.Now())
	if err := ih.db.Create(&injected).Error; err != nil {
		t.Fatalf("insert known-token invite: %v", err)
	}

	rr = ih.do(t, http.MethodGet, fmt.Sprintf("/v1/invites/%s", plaintext), "", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("preview: got %d: %s", rr.Code, rr.Body.String())
	}
	var prev invitePreviewDTO
	_ = json.NewDecoder(rr.Body).Decode(&prev)
	if prev.Email != inviteeEmail || prev.Role != "viewer" {
		t.Fatalf("bad preview: %+v", prev)
	}

	otherID := ih.createUser(t, fmt.Sprintf("other-%s@test.local", randomSuffix()), "Other")
	otherTok := ih.issueToken(t, otherID.String(), "", "")
	rr = ih.do(t, http.MethodPost, fmt.Sprintf("/v1/invites/%s/accept", plaintext), "", otherTok)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("accept mismatch: expected 403, got %d: %s", rr.Code, rr.Body.String())
	}

	inviteeTok := ih.issueToken(t, inviteeID.String(), "", "")
	rr = ih.do(t, http.MethodPost, fmt.Sprintf("/v1/invites/%s/accept", plaintext), "", inviteeTok)
	if rr.Code != http.StatusOK {
		t.Fatalf("accept: got %d: %s", rr.Code, rr.Body.String())
	}

	var m model.OrgMembership
	if err := ih.db.Where("user_id = ? AND org_id = ?", inviteeID, orgID).First(&m).Error; err != nil {
		t.Fatalf("membership not created: %v", err)
	}
	if m.Role != "viewer" {
		t.Fatalf("membership role: got %s", m.Role)
	}

	rr = ih.do(t, http.MethodGet, fmt.Sprintf("/v1/invites/%s", plaintext), "", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("preview after accept: expected 404, got %d", rr.Code)
	}
}
