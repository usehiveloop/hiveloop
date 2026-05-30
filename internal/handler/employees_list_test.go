package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
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
	h.seedSandbox(t, m, emp.ID)

	skill := model.Skill{
		Slug: "list-skill-" + randSuffix(),
		Name: "List Skill", SourceType: model.SkillSourceInline,
		Status: model.SkillStatusPublished,
	}
	if err := h.db.Create(&skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", skill.ID).Delete(&model.Skill{}) })
	if err := h.db.Create(&model.EmployeeSkill{EmployeeID: emp.ID, SkillID: skill.ID}).Error; err != nil {
		t.Fatalf("attach skill: %v", err)
	}
	t.Cleanup(func() { h.db.Where("employee_id = ?", emp.ID).Delete(&model.EmployeeSkill{}) })

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
	specialists := item["specialists"].([]any)
	if len(specialists) != 2 {
		t.Errorf("specialists len = %d, want 2", len(specialists))
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

	if _, exposed := item["profiles"]; exposed {
		t.Fatalf("employee list response exposed removed profiles field: %#v", item["profiles"])
	}
	for _, key := range []string{"is_employee", "category", "system_prompt", "identity_prompt", "prompt_operating_principles", "integrations", "agent_config"} {
		if _, exposed := item[key]; exposed {
			t.Fatalf("employee list response exposed removed %s field", key)
		}
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
	h.setSandboxSnapshot(t, matchingSandbox.ID, &h.cfg.SandboxesRuntimeBaseImage)

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
