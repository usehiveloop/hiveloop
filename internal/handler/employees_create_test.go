package handler_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestIntegration_EmployeesCreate_Engineering_Crof_HappyPath(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	crof := h.seedSystemCred(t, "crof", false)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	resp := decodeEmployeeResp(t, rr)
	if resp["agent_id"] == "" || resp["sandbox_id"] == "" {
		t.Fatalf("response missing ids: %v", resp)
	}
	if resp["status"] != "running" {
		t.Errorf("status = %q, want running", resp["status"])
	}

	var agent model.Agent
	if err := h.db.Where("id = ?", resp["agent_id"]).First(&agent).Error; err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if !agent.IsEmployee {
		t.Errorf("agent.is_employee = false, want true")
	}
	if agent.Harness != "hermes" {
		t.Errorf("agent.harness = %q, want hermes", agent.Harness)
	}
	if agent.Model != "deepseek-v4-pro-precision" {
		t.Errorf("agent.model = %q, want deepseek-v4-pro-precision (crof)", agent.Model)
	}
	if agent.CredentialID == nil || *agent.CredentialID != crof.ID {
		t.Errorf("agent.credential_id = %v, want %v (crof)", agent.CredentialID, crof.ID)
	}
	if agent.SystemPrompt == "" {
		t.Errorf("agent.system_prompt should be set to engineering placeholder")
	}
	if agent.Status != "active" {
		t.Errorf("agent.status = %q, want active", agent.Status)
	}
	if agent.Category == nil || *agent.Category != "engineering" {
		t.Errorf("agent.category = %v, want engineering", agent.Category)
	}

	var sb model.Sandbox
	if err := h.db.Where("id = ?", resp["sandbox_id"]).First(&sb).Error; err != nil {
		t.Fatalf("load sandbox: %v", err)
	}
	if sb.AgentID == nil || *sb.AgentID != agent.ID {
		t.Errorf("sandbox.agent_id mismatch")
	}
}

func TestIntegration_EmployeesCreate_FallsBackToOpenrouter(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	or := h.seedSystemCred(t, "openrouter", false)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	resp := decodeEmployeeResp(t, rr)

	var agent model.Agent
	h.db.Where("id = ?", resp["agent_id"]).First(&agent)
	if agent.Model != "deepseek/deepseek-v4-pro" {
		t.Errorf("agent.model = %q, want deepseek/deepseek-v4-pro (openrouter)", agent.Model)
	}
	if agent.CredentialID == nil || *agent.CredentialID != or.ID {
		t.Errorf("agent.credential_id mismatch: got %v want %v", agent.CredentialID, or.ID)
	}
}

func TestIntegration_EmployeesCreate_PrefersCrofWhenBothPresent(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	crof := h.seedSystemCred(t, "crof", false)
	h.seedSystemCred(t, "openrouter", false)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	resp := decodeEmployeeResp(t, rr)

	var agent model.Agent
	h.db.Where("id = ?", resp["agent_id"]).First(&agent)
	if agent.Model != "deepseek-v4-pro-precision" {
		t.Errorf("crof should win: agent.model = %q", agent.Model)
	}
	if agent.CredentialID == nil || *agent.CredentialID != crof.ID {
		t.Errorf("crof should win: agent.credential_id = %v", agent.CredentialID)
	}
}

func TestIntegration_EmployeesCreate_SkipsRevokedCrof(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "crof", true) // revoked
	or := h.seedSystemCred(t, "openrouter", false)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	resp := decodeEmployeeResp(t, rr)

	var agent model.Agent
	h.db.Where("id = ?", resp["agent_id"]).First(&agent)
	if agent.Model != "deepseek/deepseek-v4-pro" {
		t.Errorf("revoked crof must be skipped: agent.model = %q", agent.Model)
	}
	if agent.CredentialID == nil || *agent.CredentialID != or.ID {
		t.Errorf("revoked crof must be skipped: cred = %v", agent.CredentialID)
	}
}

func TestIntegration_EmployeesCreate_NoSystemCredential_503(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rr.Code, rr.Body.String())
	}

	var count int64
	h.db.Model(&model.Agent{}).Where("org_id = ?", org.org.ID).Count(&count)
	if count != 0 {
		t.Errorf("agent rows after 503 = %d, want 0", count)
	}
}

func TestIntegration_EmployeesCreate_NonEngineeringCategory_400(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "crof", false)

	body := validEmployeeBody()
	body["category"] = "design"
	rr := h.post(t, org, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}

	var count int64
	h.db.Model(&model.Agent{}).Where("org_id = ?", org.org.ID).Count(&count)
	if count != 0 {
		t.Errorf("agent rows after 400 = %d, want 0", count)
	}
}

func TestIntegration_EmployeesCreate_InvalidCategory_400(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "crof", false)

	body := validEmployeeBody()
	body["category"] = "not-a-real-category"
	rr := h.post(t, org, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesCreate_MissingFields_400(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "crof", false)

	cases := map[string]map[string]any{
		"missing name":        {"category": "engineering", "description": "desc"},
		"missing description": {"category": "engineering", "name": "n-" + uuid.NewString()[:8]},
		"missing category":    {"name": "n-" + uuid.NewString()[:8], "description": "desc"},
		"empty name":          {"category": "engineering", "name": "", "description": "desc"},
	}
	for label, body := range cases {
		t.Run(label, func(t *testing.T) {
			rr := h.post(t, org, body)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestIntegration_EmployeesCreate_RollbackOnSandboxFailure(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "crof", false)
	h.provider.failOnCreate = true

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rr.Code, rr.Body.String())
	}

	var agentCount, sbCount int64
	h.db.Model(&model.Agent{}).Where("org_id = ?", org.org.ID).Count(&agentCount)
	h.db.Model(&model.Sandbox{}).Where("org_id = ?", org.org.ID).Count(&sbCount)
	if agentCount != 0 {
		t.Errorf("agent rows after rollback = %d, want 0", agentCount)
	}
	if sbCount != 0 {
		t.Errorf("sandbox rows after rollback = %d, want 0", sbCount)
	}
}
