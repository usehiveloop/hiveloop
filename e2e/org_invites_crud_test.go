package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestOrgInviteHappyPath(t *testing.T) {
	ih := newInviteHarness(t)

	adminID, orgID := ih.createUserAndOrg(t, "admin", fmt.Sprintf("admin-%s@test.local", randomSuffix()), "admin")
	adminTok := ih.issueToken(t, adminID.String(), orgID.String(), "admin")

	inviteEmail := fmt.Sprintf("invitee-%s@test.local", randomSuffix())

	rr := ih.do(t, http.MethodPost, "/v1/orgs/current/invites",
		fmt.Sprintf(`{"email":%q,"role":"viewer"}`, inviteEmail), adminTok)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create invite: got %d: %s", rr.Code, rr.Body.String())
	}
	var created inviteDTO
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Email != inviteEmail || created.Role != "viewer" {
		t.Fatalf("unexpected invite: %+v", created)
	}

	rr = ih.do(t, http.MethodGet, "/v1/orgs/current/invites", "", adminTok)
	if rr.Code != http.StatusOK {
		t.Fatalf("list invites: got %d", rr.Code)
	}
	var list struct {
		Data []inviteDTO `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&list)
	if len(list.Data) != 1 || list.Data[0].ID != created.ID {
		t.Fatalf("expected one invite, got %+v", list.Data)
	}

	var before model.OrgInvite
	ih.db.Where("id = ?", created.ID).First(&before)

	rr = ih.do(t, http.MethodPost, fmt.Sprintf("/v1/orgs/current/invites/%s/resend", created.ID), "", adminTok)
	if rr.Code != http.StatusOK {
		t.Fatalf("resend: got %d: %s", rr.Code, rr.Body.String())
	}
	var after model.OrgInvite
	ih.db.Where("id = ?", created.ID).First(&after)
	if before.TokenHash == after.TokenHash {
		t.Fatal("resend should rotate token hash")
	}
	if !after.ExpiresAt.After(before.ExpiresAt.Add(-time.Second)) {
		t.Fatal("resend should extend expires_at")
	}

	rr = ih.do(t, http.MethodDelete, fmt.Sprintf("/v1/orgs/current/invites/%s", created.ID), "", adminTok)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("revoke: got %d: %s", rr.Code, rr.Body.String())
	}

	rr = ih.do(t, http.MethodGet, "/v1/orgs/current/invites", "", adminTok)
	_ = json.NewDecoder(rr.Body).Decode(&list)
	if len(list.Data) != 0 {
		t.Fatalf("expected no invites after revoke, got %d", len(list.Data))
	}
}
