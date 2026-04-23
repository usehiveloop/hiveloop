package e2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestOrgInviteDecline(t *testing.T) {
	ih := newInviteHarness(t)

	adminID, orgID := ih.createUserAndOrg(t, "admin", fmt.Sprintf("admin-%s@test.local", randomSuffix()), "admin")

	inviteeEmail := fmt.Sprintf("decliner-%s@test.local", randomSuffix())
	inviteeID := ih.createUser(t, inviteeEmail, "Decliner")

	plaintext, hash, _ := model.GenerateInviteToken()
	inv := model.OrgInvite{
		ID: uuid.New(), OrgID: orgID, Email: inviteeEmail, Role: "admin",
		TokenHash: hash, InvitedByID: adminID, ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := ih.db.Create(&inv).Error; err != nil {
		t.Fatalf("insert invite: %v", err)
	}

	tok := ih.issueToken(t, inviteeID.String(), "", "")
	rr := ih.do(t, http.MethodPost, fmt.Sprintf("/v1/invites/%s/decline", plaintext), "", tok)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("decline: got %d: %s", rr.Code, rr.Body.String())
	}

	var reloaded model.OrgInvite
	ih.db.Where("id = ?", inv.ID).First(&reloaded)
	if reloaded.RevokedAt == nil {
		t.Fatal("decline should set revoked_at")
	}

	rr = ih.do(t, http.MethodGet, fmt.Sprintf("/v1/invites/%s", plaintext), "", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("preview after decline: got %d", rr.Code)
	}
}
