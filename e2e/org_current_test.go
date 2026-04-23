package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestOrgCurrent(t *testing.T) {
	oh := newOrgHarness(t)

	suffix := uuid.New().String()[:8]
	email := fmt.Sprintf("orgcur-%s@test.local", suffix)
	regResp := oh.registerUser(t, email, "password123", "CurrentUser")

	t.Cleanup(func() {
		oh.db.Where("user_id = ?", regResp.User.ID).Delete(&model.OrgMembership{})
		for _, o := range regResp.Orgs {
			oh.db.Where("id = ?", o.ID).Delete(&model.Org{})
		}
		oh.db.Where("id = ?", regResp.User.ID).Delete(&model.User{})
	})

	orgName := fmt.Sprintf("e2e-current-%s", uuid.New().String()[:8])
	body := fmt.Sprintf(`{"name":%q}`, orgName)

	createRR := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body, regResp.AccessToken)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("POST /v1/orgs: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}

	var created struct {
		ID string `json:"id"`
	}
	json.NewDecoder(createRR.Body).Decode(&created)

	t.Cleanup(func() {
		oh.db.Where("org_id = ?", created.ID).Delete(&model.OrgMembership{})
		oh.db.Where("id = ?", created.ID).Delete(&model.Org{})
	})

	orgTok := oh.issueToken(t, regResp.User.ID, created.ID, "admin")

	rr := oh.orgRequest(t, http.MethodGet, "/v1/orgs/current", "", orgTok)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /v1/orgs/current: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.ID != created.ID {
		t.Errorf("id: got %q, want %q", resp.ID, created.ID)
	}
	if resp.Name != orgName {
		t.Errorf("name: got %q, want %q", resp.Name, orgName)
	}
	if !resp.Active {
		t.Error("org should be active")
	}
}

func TestOrgCreateDuplicateName(t *testing.T) {
	oh := newOrgHarness(t)

	suffix := uuid.New().String()[:8]
	email := fmt.Sprintf("orgdup-%s@test.local", suffix)
	regResp := oh.registerUser(t, email, "password123", "DupTester")

	t.Cleanup(func() {
		oh.db.Where("user_id = ?", regResp.User.ID).Delete(&model.OrgMembership{})
		for _, o := range regResp.Orgs {
			oh.db.Where("id = ?", o.ID).Delete(&model.Org{})
		}
		oh.db.Where("id = ?", regResp.User.ID).Delete(&model.User{})
	})

	tok := regResp.AccessToken

	orgName := fmt.Sprintf("e2e-dup-%s", uuid.New().String()[:8])
	body := fmt.Sprintf(`{"name":%q}`, orgName)

	rr1 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body, tok)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var org1 struct{ ID string }
	json.NewDecoder(rr1.Body).Decode(&org1)

	t.Cleanup(func() {
		oh.db.Where("org_id = ?", org1.ID).Delete(&model.OrgMembership{})
		oh.db.Where("id = ?", org1.ID).Delete(&model.Org{})
	})

	rr2 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body, tok)
	if rr2.Code != http.StatusInternalServerError {
		t.Errorf("duplicate name: expected 500, got %d: %s", rr2.Code, rr2.Body.String())
	}
}

func TestOrgCreateMultiple(t *testing.T) {
	oh := newOrgHarness(t)

	suffix := uuid.New().String()[:8]
	email := fmt.Sprintf("orgmulti-%s@test.local", suffix)
	regResp := oh.registerUser(t, email, "password123", "MultiTester")

	t.Cleanup(func() {
		oh.db.Where("user_id = ?", regResp.User.ID).Delete(&model.OrgMembership{})
		for _, o := range regResp.Orgs {
			oh.db.Where("id = ?", o.ID).Delete(&model.Org{})
		}
		oh.db.Where("id = ?", regResp.User.ID).Delete(&model.User{})
	})

	tok := regResp.AccessToken

	name1 := fmt.Sprintf("e2e-multi-a-%s", uuid.New().String()[:8])
	name2 := fmt.Sprintf("e2e-multi-b-%s", uuid.New().String()[:8])

	rr1 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", fmt.Sprintf(`{"name":%q}`, name1), tok)
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", rr1.Code, rr1.Body.String())
	}
	var org1 struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr1.Body).Decode(&org1)

	rr2 := oh.orgRequest(t, http.MethodPost, "/v1/orgs", fmt.Sprintf(`{"name":%q}`, name2), tok)
	if rr2.Code != http.StatusCreated {
		t.Fatalf("second create: expected 201, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var org2 struct {
		ID string `json:"id"`
	}
	json.NewDecoder(rr2.Body).Decode(&org2)

	if org1.ID == org2.ID {
		t.Error("two orgs should have different IDs")
	}

	t.Cleanup(func() {
		oh.db.Where("org_id IN ?", []string{org1.ID, org2.ID}).Delete(&model.OrgMembership{})
		oh.db.Where("id IN ?", []string{org1.ID, org2.ID}).Delete(&model.Org{})
	})
}
