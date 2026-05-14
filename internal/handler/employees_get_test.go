package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func (h *employeeHarness) getEmployee(t *testing.T, m orgWithMember, agentID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/v1/employees/"+agentID, nil)
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

func TestIntegration_EmployeesGet_HappyPath_LoadsProfilesSubagentsAndSandbox(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, emp.ID)

	profile := model.AgentProfile{
		OrgID:      m.org.ID,
		AgentID:    emp.ID,
		Provider:   "github",
		ExternalID: "conn-github",
		Label:      "GitHub",
		Status:     "active",
		Config: model.JSON{
			"selected_repositories": []any{
				map[string]any{
					"id":        "repo-1",
					"full_name": "usehiveloop/hiveloop.com",
				},
			},
		},
	}
	if err := h.db.Create(&profile).Error; err != nil {
		t.Fatalf("create github profile: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", profile.ID).Delete(&model.AgentProfile{}) })

	subagent := model.Agent{
		OrgID: &m.org.ID, Name: "research-" + uuid.NewString()[:6],
		IsEmployee: false, Status: "active", SystemPrompt: "x", Model: "y",
	}
	if err := h.db.Create(&subagent).Error; err != nil {
		t.Fatalf("create subagent: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", subagent.ID).Delete(&model.Agent{}) })
	if err := h.db.Create(&model.AgentSubagent{AgentID: emp.ID, SubagentID: subagent.ID}).Error; err != nil {
		t.Fatalf("link subagent: %v", err)
	}
	t.Cleanup(func() { h.db.Where("agent_id = ?", emp.ID).Delete(&model.AgentSubagent{}) })

	rr := h.getEmployee(t, m, emp.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var item map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if item["id"] != emp.ID.String() {
		t.Fatalf("id = %v, want %s", item["id"], emp.ID)
	}
	profiles := item["profiles"].([]any)
	if len(profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(profiles))
	}
	github := profiles[0].(map[string]any)
	if github["provider"] != "github" || github["status"] != "active" {
		t.Fatalf("github profile = %#v", github)
	}
	config := github["config"].(map[string]any)
	selected := config["selected_repositories"].([]any)
	if len(selected) != 1 {
		t.Fatalf("selected repositories len = %d, want 1", len(selected))
	}
	if _, ok := item["sandbox"].(map[string]any); !ok {
		t.Fatalf("sandbox missing: %#v", item["sandbox"])
	}
	subagents := item["subagents"].([]any)
	if len(subagents) != 1 {
		t.Fatalf("subagents len = %d, want 1", len(subagents))
	}
}

func TestIntegration_EmployeesGet_RejectsNonEmployee(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := model.Agent{
		OrgID: &m.org.ID, Name: "plain-agent-" + uuid.NewString()[:6],
		IsEmployee: false, Status: "active", SystemPrompt: "x", Model: "y",
	}
	if err := h.db.Create(&agent).Error; err != nil {
		t.Fatalf("create plain agent: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })

	rr := h.getEmployee(t, m, agent.ID.String())
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesGet_ScopedToOrg(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	owner := h.createOrg(t)
	stranger := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, owner)

	rr := h.getEmployee(t, stranger, emp.ID.String())
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rr.Code, rr.Body.String())
	}
}
