package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *employeeHarness) postSync(t *testing.T, m orgWithMember, agentID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/employees/"+agentID+"/sync", bytes.NewReader(nil))
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

func TestIntegration_EmployeesSync_Slack_HappyPath(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["applied"].(float64) != 3 {
		t.Errorf("applied = %v, want 3", resp["applied"])
	}
	if resp["repos_cloned"].(float64) != 1 {
		t.Errorf("repos_cloned = %v, want 1", resp["repos_cloned"])
	}
	if resp["restart_triggered"] != true {
		t.Errorf("restart_triggered = %v, want true", resp["restart_triggered"])
	}

	calls, bearer := h.sidecar.snapshot()
	if calls != 1 {
		t.Errorf("sidecar /v1/config/sync called %d times, want 1", calls)
	}
	if bearer == "" || bearer == "Bearer " {
		t.Errorf("sidecar bearer header missing: %q", bearer)
	}

	// Compile must have minted a 'hermes' token tagged with the agent.
	var tokenCount int64
	h.db.Model(&model.Token{}).
		Where("meta->>'agent_id' = ? AND meta->>'harness' = ?", agent.ID.String(), "hermes").
		Count(&tokenCount)
	if tokenCount != 1 {
		t.Errorf("hermes token rows = %d, want 1", tokenCount)
	}
}

func TestIntegration_EmployeesSync_Whatsapp_HappyPath(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedWhatsappProfile(t, m, agent.ID)

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesSync_NotEmployee_400(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	cred := h.seedSystemCred(t, "crof", false)
	credID := cred.ID
	agent := model.Agent{
		OrgID:        &m.org.ID,
		Name:         "not-employee",
		IsEmployee:   false,
		Harness:      "hermes",
		Model:        "deepseek-v4-pro-precision",
		SystemPrompt: "x",
		CredentialID: &credID,
		Status:       "active",
	}
	if err := h.db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
	calls, _ := h.sidecar.snapshot()
	if calls != 0 {
		t.Errorf("sidecar called %d times, want 0", calls)
	}
}

func TestIntegration_EmployeesSync_NoActiveProfile_400(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesSync_RevokedProfile_400(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	p := h.seedSlackProfile(t, m, agent.ID)
	if err := h.db.Model(&p).Update("status", "revoked").Error; err != nil {
		t.Fatalf("revoke profile: %v", err)
	}

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (revoked profile must not satisfy gate): %s",
			rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesSync_NoSandbox_409(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSlackProfile(t, m, agent.ID)
	// no sandbox seeded

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesSync_OtherOrg_404(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	owner := h.createOrg(t)
	intruder := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, owner)
	h.seedSandbox(t, owner, agent.ID)
	h.seedSlackProfile(t, owner, agent.ID)

	rr := h.postSync(t, intruder, agent.ID.String())
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (cross-org access): %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesSync_AgentNotFound_404(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)

	rr := h.postSync(t, m, uuid.NewString())
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesSync_InvalidUUID_400(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)

	rr := h.postSync(t, m, "not-a-uuid")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesSync_NonAdmin_403(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrgWithRole(t, "member")
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rr.Code, rr.Body.String())
	}
	calls, _ := h.sidecar.snapshot()
	if calls != 0 {
		t.Errorf("sidecar called %d times, want 0 (request gated)", calls)
	}
}

func TestIntegration_EmployeesSync_SidecarRejects_502(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)
	h.sidecar.setStatus(http.StatusInternalServerError)

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502: %s", rr.Code, rr.Body.String())
	}
}
