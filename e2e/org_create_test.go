package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestOrgCreate(t *testing.T) {
	oh := newOrgHarness(t)

	suffix := uuid.New().String()[:8]
	email := fmt.Sprintf("orgcreate-%s@test.local", suffix)
	regResp := oh.registerUser(t, email, "password123", "OrgCreator")

	t.Cleanup(func() {
		oh.db.Where("user_id = ?", regResp.User.ID).Delete(&model.OrgMembership{})
		oh.db.Where("id = ?", regResp.User.ID).Delete(&model.User{})
		for _, o := range regResp.Orgs {
			oh.db.Where("id = ?", o.ID).Delete(&model.Org{})
		}
	})

	orgName := fmt.Sprintf("e2e-org-%s", uuid.New().String()[:8])
	body := fmt.Sprintf(`{"name":%q}`, orgName)

	rr := oh.orgRequest(t, http.MethodPost, "/v1/orgs", body, regResp.AccessToken)
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /v1/orgs: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		RateLimit int    `json:"rate_limit"`
		Active    bool   `json:"active"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	t.Cleanup(func() {
		oh.db.Where("org_id = ?", resp.ID).Delete(&model.OrgMembership{})
		oh.db.Where("id = ?", resp.ID).Delete(&model.Org{})
	})

	if resp.Name != orgName {
		t.Errorf("name: got %q, want %q", resp.Name, orgName)
	}
	if resp.ID == "" {
		t.Error("id is empty")
	}
	if !resp.Active {
		t.Error("org should be active")
	}
	if resp.RateLimit != 1000 {
		t.Errorf("rate_limit: got %d, want 1000 (default)", resp.RateLimit)
	}

	var dbOrg model.Org
	if err := oh.db.Where("id = ?", resp.ID).First(&dbOrg).Error; err != nil {
		t.Fatalf("org not found in DB: %v", err)
	}
	if dbOrg.Name != orgName {
		t.Errorf("DB name mismatch: got %q, want %q", dbOrg.Name, orgName)
	}
}

func TestOrgCreateValidation(t *testing.T) {
	oh := newOrgHarness(t)

	suffix := uuid.New().String()[:8]
	email := fmt.Sprintf("orgval-%s@test.local", suffix)
	regResp := oh.registerUser(t, email, "password123", "Validator")

	t.Cleanup(func() {
		oh.db.Where("user_id = ?", regResp.User.ID).Delete(&model.OrgMembership{})
		for _, o := range regResp.Orgs {
			oh.db.Where("id = ?", o.ID).Delete(&model.Org{})
		}
		oh.db.Where("id = ?", regResp.User.ID).Delete(&model.User{})
	})

	tok := regResp.AccessToken

	rr := oh.orgRequest(t, http.MethodPost, "/v1/orgs", `{"name":""}`, tok)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("empty name: expected 400, got %d", rr.Code)
	}

	rr = oh.orgRequest(t, http.MethodPost, "/v1/orgs", `not json`, tok)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("invalid json: expected 400, got %d", rr.Code)
	}
}

func TestOrgCreateUnauthenticated(t *testing.T) {
	oh := newOrgHarness(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/orgs", strings.NewReader(`{"name":"nope"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	oh.orgRouter.ServeHTTP(rr, req)

	if rr.Code == http.StatusCreated {
		t.Error("expected unauthenticated request to fail, got 201")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}
