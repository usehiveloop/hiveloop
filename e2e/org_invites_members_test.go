package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestOrgInviteListMembers(t *testing.T) {
	ih := newInviteHarness(t)

	adminID, orgID := ih.createUserAndOrg(t, "admin", fmt.Sprintf("admin-%s@test.local", randomSuffix()), "admin")
	adminTok := ih.issueToken(t, adminID.String(), orgID.String(), "admin")

	rr := ih.do(t, http.MethodGet, "/v1/orgs/current/members", "", adminTok)
	if rr.Code != http.StatusOK {
		t.Fatalf("list members: got %d: %s", rr.Code, rr.Body.String())
	}
	var list struct {
		Data []struct {
			UserID string `json:"user_id"`
			Email  string `json:"email"`
			Role   string `json:"role"`
		} `json:"data"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&list)
	if len(list.Data) != 1 || list.Data[0].UserID != adminID.String() || list.Data[0].Role != "admin" {
		t.Fatalf("unexpected members list: %+v", list.Data)
	}
}
