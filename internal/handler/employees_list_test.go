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

func (h *employeeHarness) listEmployees(t *testing.T, m orgWithMember) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/v1/employees", nil)
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

func TestIntegration_EmployeesList_HappyPath_LoadsAllRelations(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, m)
	team := model.Team{
		OrgID:       m.org.ID,
		Name:        "Engineering",
		Description: "AI engineering team.",
	}
	if err := h.db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", team.ID).Delete(&model.Team{}) })
	if err := h.db.Model(&model.Agent{}).
		Where("id = ?", emp.ID).
		Update("team_id", team.ID).Error; err != nil {
		t.Fatalf("assign team: %v", err)
	}
	h.seedSandbox(t, m, emp.ID)
	h.seedSlackProfile(t, m, emp.ID)

	subagent := model.Agent{
		OrgID: &m.org.ID, Name: "sub-" + uuid.NewString()[:6],
		IsEmployee: false, Status: "active", SystemPrompt: "x", Model: "y",
	}
	if err := h.db.Create(&subagent).Error; err != nil {
		t.Fatalf("create subagent: %v", err)
	}
	subID := subagent.ID
	t.Cleanup(func() { h.db.Where("id = ?", subID).Delete(&model.Agent{}) })
	if err := h.db.Create(&model.AgentSubagent{AgentID: emp.ID, SubagentID: subID}).Error; err != nil {
		t.Fatalf("link subagent: %v", err)
	}
	t.Cleanup(func() { h.db.Where("agent_id = ?", emp.ID).Delete(&model.AgentSubagent{}) })

	skill := model.Skill{
		ID: uuid.New(), Slug: "list-skill-" + uuid.NewString()[:6],
		Name: "List Skill", SourceType: model.SkillSourceInline,
		Status: model.SkillStatusPublished,
	}
	if err := h.db.Create(&skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", skill.ID).Delete(&model.Skill{}) })
	if err := h.db.Create(&model.AgentSkill{AgentID: emp.ID, SkillID: skill.ID}).Error; err != nil {
		t.Fatalf("attach skill: %v", err)
	}
	t.Cleanup(func() { h.db.Where("agent_id = ?", emp.ID).Delete(&model.AgentSkill{}) })

	rr := h.listEmployees(t, m)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("data len = %d, want 1", len(resp.Data))
	}
	item := resp.Data[0]

	if item["id"] != emp.ID.String() {
		t.Errorf("id mismatch: got %v", item["id"])
	}
	if item["is_employee"] != true {
		t.Errorf("is_employee = %v, want true", item["is_employee"])
	}
	if item["team"] != "Engineering" {
		t.Errorf("team = %v, want Engineering", item["team"])
	}

	subagents := item["subagents"].([]any)
	if len(subagents) != 1 {
		t.Errorf("subagents len = %d, want 1", len(subagents))
	} else {
		sa := subagents[0].(map[string]any)
		if sa["id"] != subID.String() {
			t.Errorf("subagent id mismatch: got %v", sa["id"])
		}
	}

	attached := item["attached_skills"].([]any)
	if len(attached) != 1 {
		t.Errorf("attached_skills len = %d, want 1", len(attached))
	} else {
		sk := attached[0].(map[string]any)
		if sk["name"] != "List Skill" {
			t.Errorf("skill name = %v, want List Skill", sk["name"])
		}
		// Skill summary must NOT carry bundle content.
		if _, hasContent := sk["content"]; hasContent {
			t.Errorf("skill summary leaked content field")
		}
	}

	profiles := item["profiles"].([]any)
	if len(profiles) != 1 {
		t.Errorf("profiles len = %d, want 1", len(profiles))
	}

	sb := item["sandbox"].(map[string]any)
	if sb["status"] != "running" {
		t.Errorf("sandbox.status = %v, want running", sb["status"])
	}
}

func TestIntegration_EmployeesList_ReportsSandboxUpgradeAvailability(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)

	matching := h.seedEmployeeAgent(t, m)
	matchingSandbox := h.seedSandbox(t, m, matching.ID)
	h.setSandboxSnapshot(t, matchingSandbox.ID, &h.cfg.EmployeeSandboxBaseImagePrefix)

	outdated := h.seedEmployeeAgent(t, m)
	outdatedSandbox := h.seedSandbox(t, m, outdated.ID)
	outdatedSnapshot := "older-employee-sandbox"
	h.setSandboxSnapshot(t, outdatedSandbox.ID, &outdatedSnapshot)

	legacy := h.seedEmployeeAgent(t, m)
	legacySandbox := h.seedSandbox(t, m, legacy.ID)
	h.setSandboxSnapshot(t, legacySandbox.ID, nil)

	rr := h.listEmployees(t, m)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := make(map[string]bool, len(resp.Data))
	for _, item := range resp.Data {
		if _, exposed := item["snapshot_id"]; exposed {
			t.Fatalf("employee response exposed snapshot_id: %#v", item)
		}
		if sandbox, ok := item["sandbox"].(map[string]any); ok {
			if _, exposed := sandbox["snapshot_id"]; exposed {
				t.Fatalf("employee sandbox response exposed snapshot_id: %#v", sandbox)
			}
		}
		id, _ := item["id"].(string)
		upgrade, _ := item["upgrade_available"].(bool)
		got[id] = upgrade
	}

	if got[matching.ID.String()] {
		t.Errorf("matching sandbox upgrade_available = true, want false")
	}
	if !got[outdated.ID.String()] {
		t.Errorf("outdated sandbox upgrade_available = false, want true")
	}
	if !got[legacy.ID.String()] {
		t.Errorf("legacy sandbox upgrade_available = false, want true")
	}
}

func TestIntegration_EmployeesList_ExcludesNonEmployees(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, m)

	notEmp := model.Agent{
		OrgID: &m.org.ID, Name: "plain-agent-" + uuid.NewString()[:6],
		IsEmployee: false, Status: "active", SystemPrompt: "x", Model: "y",
	}
	if err := h.db.Create(&notEmp).Error; err != nil {
		t.Fatalf("create plain agent: %v", err)
	}
	notEmpID := notEmp.ID
	t.Cleanup(func() { h.db.Where("id = ?", notEmpID).Delete(&model.Agent{}) })

	rr := h.listEmployees(t, m)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Data) != 1 {
		t.Fatalf("data len = %d, want 1 (only the employee)", len(resp.Data))
	}
	if resp.Data[0]["id"] != emp.ID.String() {
		t.Errorf("returned wrong agent: %v", resp.Data[0]["id"])
	}
}

func TestIntegration_EmployeesList_ScopedToOrg(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	owner := h.createOrg(t)
	stranger := h.createOrg(t)
	h.seedEmployeeAgent(t, owner)

	rr := h.listEmployees(t, stranger)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Data) != 0 {
		t.Fatalf("cross-org: data len = %d, want 0", len(resp.Data))
	}
}

func TestIntegration_EmployeesList_NonAdminAllowed(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrgWithRole(t, "member")
	h.seedEmployeeAgent(t, m)

	rr := h.listEmployees(t, m)
	if rr.Code != http.StatusOK {
		t.Fatalf("non-admin should read list: status = %d, body=%s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesList_EmptyOrg(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)

	rr := h.listEmployees(t, m)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp struct {
		Data    []any `json:"data"`
		HasMore bool  `json:"has_more"`
	}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Data) != 0 {
		t.Errorf("empty org: data len = %d, want 0", len(resp.Data))
	}
	if resp.HasMore {
		t.Errorf("empty org: has_more = true, want false")
	}
}
