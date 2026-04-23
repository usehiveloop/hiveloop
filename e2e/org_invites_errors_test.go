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

func TestOrgInviteErrorCases(t *testing.T) {
	ih := newInviteHarness(t)

	adminID, orgID := ih.createUserAndOrg(t, "admin", fmt.Sprintf("admin-%s@test.local", randomSuffix()), "admin")
	adminTok := ih.issueToken(t, adminID.String(), orgID.String(), "admin")

	rr := ih.do(t, http.MethodPost, "/v1/orgs/current/invites", `{"email":"not-an-email","role":"viewer"}`, adminTok)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid email: expected 400, got %d", rr.Code)
	}

	rr = ih.do(t, http.MethodPost, "/v1/orgs/current/invites", `{"email":"x@y.z","role":"owner"}`, adminTok)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid role: expected 400, got %d", rr.Code)
	}

	viewerEmail := fmt.Sprintf("viewer-%s@test.local", randomSuffix())
	viewerID := ih.createUser(t, viewerEmail, "Viewer")
	viewerMembership := model.OrgMembership{UserID: viewerID, OrgID: orgID, Role: "viewer"}
	if err := ih.db.Create(&viewerMembership).Error; err != nil {
		t.Fatalf("create viewer membership: %v", err)
	}
	viewerTok := ih.issueToken(t, viewerID.String(), orgID.String(), "viewer")
	rr = ih.do(t, http.MethodPost, "/v1/orgs/current/invites", `{"email":"someone@test.local","role":"viewer"}`, viewerTok)
	if rr.Code != http.StatusForbidden {
		t.Errorf("non-admin invite: expected 403, got %d", rr.Code)
	}
	rr = ih.do(t, http.MethodGet, "/v1/orgs/current/invites", "", viewerTok)
	if rr.Code != http.StatusForbidden {
		t.Errorf("non-admin list: expected 403, got %d", rr.Code)
	}

	existingEmail := fmt.Sprintf("member-%s@test.local", randomSuffix())
	existingID := ih.createUser(t, existingEmail, "Already")
	if err := ih.db.Create(&model.OrgMembership{UserID: existingID, OrgID: orgID, Role: "viewer"}).Error; err != nil {
		t.Fatalf("create existing membership: %v", err)
	}
	rr = ih.do(t, http.MethodPost, "/v1/orgs/current/invites",
		fmt.Sprintf(`{"email":%q,"role":"viewer"}`, existingEmail), adminTok)
	if rr.Code != http.StatusConflict {
		t.Errorf("invite existing member: expected 409, got %d: %s", rr.Code, rr.Body.String())
	}

	dupEmail := fmt.Sprintf("dup-%s@test.local", randomSuffix())
	rr = ih.do(t, http.MethodPost, "/v1/orgs/current/invites",
		fmt.Sprintf(`{"email":%q,"role":"viewer"}`, dupEmail), adminTok)
	if rr.Code != http.StatusCreated {
		t.Fatalf("first invite: got %d", rr.Code)
	}
	rr = ih.do(t, http.MethodPost, "/v1/orgs/current/invites",
		fmt.Sprintf(`{"email":%q,"role":"viewer"}`, dupEmail), adminTok)
	if rr.Code != http.StatusConflict {
		t.Errorf("duplicate invite: expected 409, got %d", rr.Code)
	}

	expUserEmail := fmt.Sprintf("exp-%s@test.local", randomSuffix())
	expUserID := ih.createUser(t, expUserEmail, "Exp")
	plaintext, hash, _ := model.GenerateInviteToken()
	expInvite := model.OrgInvite{
		ID: uuid.New(), OrgID: orgID, Email: expUserEmail, Role: "viewer",
		TokenHash: hash, InvitedByID: adminID,
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	if err := ih.db.Create(&expInvite).Error; err != nil {
		t.Fatalf("create expired invite: %v", err)
	}
	expTok := ih.issueToken(t, expUserID.String(), "", "")
	rr = ih.do(t, http.MethodPost, fmt.Sprintf("/v1/invites/%s/accept", plaintext), "", expTok)
	if rr.Code != http.StatusNotFound {
		t.Errorf("accept expired: expected 404, got %d", rr.Code)
	}

	revPlain, revHash, _ := model.GenerateInviteToken()
	revokedAt := time.Now()
	revInvite := model.OrgInvite{
		ID: uuid.New(), OrgID: orgID, Email: fmt.Sprintf("rev-%s@test.local", randomSuffix()),
		Role: "viewer", TokenHash: revHash, InvitedByID: adminID,
		ExpiresAt: time.Now().Add(time.Hour), RevokedAt: &revokedAt,
	}
	if err := ih.db.Create(&revInvite).Error; err != nil {
		t.Fatalf("create revoked invite: %v", err)
	}
	rr = ih.do(t, http.MethodGet, fmt.Sprintf("/v1/invites/%s", revPlain), "", "")
	if rr.Code != http.StatusNotFound {
		t.Errorf("preview revoked: expected 404, got %d", rr.Code)
	}
}
