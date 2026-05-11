package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func validOnboardingBody() map[string]any {
	return map[string]any{
		"name":        "Kibamail",
		"website":     "https://kibamail.com",
		"logo_url":    "https://cdn.example/logo.png",
		"description": "Email infrastructure built for product teams.",
	}
}

func (h *employeeHarness) postCompleteOnboarding(t *testing.T, m orgWithMember, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	_ = json.NewEncoder(buf).Encode(body)
	req := httptest.NewRequest("POST", "/v1/orgs/current/onboarding/complete", buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "admin",
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestIntegration_OnboardingComplete_HappyPath(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)

	rr := h.postCompleteOnboarding(t, m, validOnboardingBody())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		AgentID string                 `json:"agent_id"`
		Sync    map[string]interface{} `json:"sync"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AgentID != agent.ID.String() {
		t.Errorf("agent_id = %q, want %s", resp.AgentID, agent.ID)
	}
	if resp.Sync["applied"].(float64) != 1 {
		t.Errorf("sync.applied = %v, want 1", resp.Sync["applied"])
	}

	calls, _ := h.sidecar.snapshot()
	if calls != 1 {
		t.Errorf("runtime /config calls = %d, want 1", calls)
	}

	var fromDB model.Org
	if err := h.db.Where("id = ?", m.org.ID).First(&fromDB).Error; err != nil {
		t.Fatalf("re-read org: %v", err)
	}
	if fromDB.Name != "Kibamail" {
		t.Errorf("org.name = %q, want Kibamail", fromDB.Name)
	}
	if fromDB.Website != "https://kibamail.com" {
		t.Errorf("org.website = %q", fromDB.Website)
	}
	if fromDB.LogoURL != "https://cdn.example/logo.png" {
		t.Errorf("org.logo_url = %q", fromDB.LogoURL)
	}
	if fromDB.Description != "Email infrastructure built for product teams." {
		t.Errorf("org.description = %q", fromDB.Description)
	}
	if !fromDB.Onboarded {
		t.Errorf("org.onboarded = false, want true after success")
	}
}

func TestIntegration_OnboardingComplete_NoEmployee_400(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)

	rr := h.postCompleteOnboarding(t, m, validOnboardingBody())
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}

	var fromDB model.Org
	h.db.Where("id = ?", m.org.ID).First(&fromDB)
	if fromDB.Onboarded {
		t.Errorf("onboarded must stay false when no employee exists")
	}
}

func TestIntegration_OnboardingComplete_NoActiveProfile_400(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)

	rr := h.postCompleteOnboarding(t, m, validOnboardingBody())
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}

	calls, _ := h.sidecar.snapshot()
	if calls != 0 {
		t.Errorf("sidecar should not be called when validation fails: %d", calls)
	}
}

func TestIntegration_OnboardingComplete_NoSandbox_Provisions(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSlackProfile(t, m, agent.ID)

	rr := h.postCompleteOnboarding(t, m, validOnboardingBody())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_OnboardingComplete_MissingName_400(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)

	body := validOnboardingBody()
	delete(body, "name")
	rr := h.postCompleteOnboarding(t, m, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_OnboardingComplete_InvalidWebsite_400(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)

	for _, bad := range []string{"javascript:alert(1)", "ftp://x", "//example", "not a url"} {
		body := validOnboardingBody()
		body["website"] = bad
		rr := h.postCompleteOnboarding(t, m, body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("website=%q: status = %d, want 400", bad, rr.Code)
		}
	}
}

func TestIntegration_OnboardingComplete_NonAdmin_403(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrgWithRole(t, "member")
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)

	rr := h.postCompleteOnboarding(t, m, validOnboardingBody())
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_OnboardingComplete_SidecarRejects_502_NotOnboarded(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)
	h.sidecar.setStatus(http.StatusInternalServerError)

	rr := h.postCompleteOnboarding(t, m, validOnboardingBody())
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", rr.Code, rr.Body.String())
	}

	var fromDB model.Org
	h.db.Where("id = ?", m.org.ID).First(&fromDB)
	if fromDB.Onboarded {
		t.Errorf("onboarded must stay false when sync fails")
	}
	if fromDB.Name != "Kibamail" {
		t.Errorf("business info should still persist on sync failure: name=%q", fromDB.Name)
	}
}
