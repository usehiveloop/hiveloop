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

func (h *employeeHarness) listAgentTemplates(t *testing.T, m orgWithMember, agentID uuid.UUID, role string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/employees/"+agentID.String()+"/agent-templates", nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   role,
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *employeeHarness) postAgentTemplateInstall(t *testing.T, m orgWithMember, agentID uuid.UUID, slug string, role string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/employees/"+agentID.String()+"/agent-templates/"+slug+"/install", bytes.NewReader(nil))
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   role,
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestIntegration_EmployeeAgentTemplates_ListShowsInstalledDefaults(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", false)

	rr := h.post(t, m, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	agentID := uuid.MustParse(decodeEmployeeResp(t, rr)["agent_id"])

	rr = h.listAgentTemplates(t, m, agentID, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	templates := decodeTemplateList(t, rr)
	if len(templates) != 2 {
		t.Fatalf("template count = %d, want 2: %#v", len(templates), templates)
	}
	for _, slug := range []string{"business-research-specialist", "software-engineering-specialist"} {
		template := templatesBySlug(templates)[slug]
		if template == nil {
			t.Fatalf("template %s missing: %#v", slug, templates)
		}
		if template.Installed != true {
			t.Fatalf("template %s installed = false, want true", slug)
		}
		if template.SubagentID == nil || *template.SubagentID == "" {
			t.Fatalf("template %s subagent_id missing", slug)
		}
		if template.Category != "engineering" {
			t.Fatalf("template %s category = %q, want engineering", slug, template.Category)
		}
	}

	rr = h.getEmployee(t, m, agentID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("get employee status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var employee map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &employee); err != nil {
		t.Fatalf("decode employee: %v", err)
	}
	subagents := employee["subagents"].([]any)
	if len(subagents) != 2 {
		t.Fatalf("subagent count = %d, want 2", len(subagents))
	}
	for _, raw := range subagents {
		subagent := raw.(map[string]any)
		if subagent["template_slug"] == "" || subagent["template_agent_type"] == "" {
			t.Fatalf("subagent missing template metadata: %#v", subagent)
		}
	}
}

func TestIntegration_EmployeeAgentTemplates_ListReadableByMember(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrgWithRole(t, "member")
	agent := h.seedEmployeeAgent(t, m)

	rr := h.listAgentTemplates(t, m, agent.ID, "member")
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200 for member: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeeAgentTemplates_InstallExistingIsIdempotentAndSyncs(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("sync status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	beforeCalls, _ := h.sidecar.snapshot()

	rr = h.postAgentTemplateInstall(t, m, agent.ID, "business-research-specialist", "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("install status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	afterCalls, _ := h.sidecar.snapshot()
	if afterCalls != beforeCalls+1 {
		t.Fatalf("sync calls after install = %d, want %d", afterCalls, beforeCalls+1)
	}

	var resp installTemplateTestResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode install response: %v", err)
	}
	if resp.Template.Slug != "business-research-specialist" || !resp.Template.Installed {
		t.Fatalf("template response = %#v, want installed business research", resp.Template)
	}
	if resp.Subagent.TemplateSlug == nil || *resp.Subagent.TemplateSlug != "business-research-specialist" {
		t.Fatalf("subagent template_slug = %v, want business-research-specialist", resp.Subagent.TemplateSlug)
	}
	if resp.Sync.Applied != 1 {
		t.Fatalf("sync.applied = %d, want 1", resp.Sync.Applied)
	}

	var linkCount int64
	if err := h.db.Model(&model.AgentSubagent{}).Where("agent_id = ?", agent.ID).Count(&linkCount).Error; err != nil {
		t.Fatalf("count links: %v", err)
	}
	if linkCount != 2 {
		t.Fatalf("link count after idempotent install = %d, want 2", linkCount)
	}
}

func TestIntegration_EmployeeAgentTemplates_InstallMissingCreatesSubagentAndSyncs(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)
	agentBrowser := h.seedGlobalSkill(t, "agent-browser", model.SkillStatusPublished)
	gitGithub := h.seedGlobalSkill(t, "git-github", model.SkillStatusPublished)
	assetUploads := h.seedGlobalSkill(t, "asset-uploads", model.SkillStatusPublished)

	rr := h.postAgentTemplateInstall(t, m, agent.ID, "software-engineering-specialist", "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("install status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	subagents := defaultSubagentsByType(t, h.db, agent.ID)
	if len(subagents) != 1 {
		t.Fatalf("subagent count = %d, want 1: %#v", len(subagents), subagents)
	}
	software, ok := subagents["software_engineering_specialist"]
	if !ok {
		t.Fatalf("software engineering specialist not created: %#v", subagents)
	}

	subLinks := skillIDsFor(t, h.db, software.ID)
	for _, id := range []uuid.UUID{agentBrowser.ID, gitGithub.ID, assetUploads.ID} {
		if !subLinks[id] {
			t.Fatalf("software subagent missing skill %v; got %v", id, subLinks)
		}
	}
	calls, _ := h.sidecar.snapshot()
	if calls != 1 {
		t.Fatalf("runtime sync calls = %d, want 1", calls)
	}
}

func TestIntegration_EmployeeAgentTemplates_InstallInvalidSlug404(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)

	rr := h.postAgentTemplateInstall(t, m, agent.ID, "not-a-template", "admin")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("install status = %d, want 404: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeeAgentTemplates_InstallRequiresAdmin(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrgWithRole(t, "member")
	agent := h.seedEmployeeAgent(t, m)

	rr := h.postAgentTemplateInstall(t, m, agent.ID, "business-research-specialist", "member")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("install status = %d, want 403: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeeAgentTemplates_InstallBlockedByActiveUpgrade(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	upgrade := model.EmployeeSandboxUpgrade{
		OrgID:   m.org.ID,
		AgentID: agent.ID,
		Status:  model.EmployeeSandboxUpgradeStatusQueued,
		Phase:   model.EmployeeSandboxUpgradePhaseQueued,
	}
	if err := h.db.Create(&upgrade).Error; err != nil {
		t.Fatalf("create upgrade: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", upgrade.ID).Delete(&model.EmployeeSandboxUpgrade{}) })

	rr := h.postAgentTemplateInstall(t, m, agent.ID, "business-research-specialist", "admin")
	if rr.Code != http.StatusConflict {
		t.Fatalf("install status = %d, want 409: %s", rr.Code, rr.Body.String())
	}
	calls, _ := h.sidecar.snapshot()
	if calls != 0 {
		t.Fatalf("runtime sync calls = %d, want 0", calls)
	}
	subagents := defaultSubagentsByType(t, h.db, agent.ID)
	if len(subagents) != 0 {
		t.Fatalf("subagents created during blocked install: %#v", subagents)
	}
}

type templateTestResponse struct {
	Slug       string  `json:"slug"`
	Category   string  `json:"category"`
	AgentType  string  `json:"agent_type"`
	Installed  bool    `json:"installed"`
	SubagentID *string `json:"subagent_id"`
}

type subagentTestResponse struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	TemplateSlug      *string `json:"template_slug"`
	TemplateAgentType *string `json:"template_agent_type"`
}

type syncTestResponse struct {
	Applied int `json:"applied"`
}

type installTemplateTestResponse struct {
	Template templateTestResponse `json:"template"`
	Subagent subagentTestResponse `json:"subagent"`
	Sync     syncTestResponse     `json:"sync"`
}

func decodeTemplateList(t *testing.T, rr *httptest.ResponseRecorder) []templateTestResponse {
	t.Helper()
	var templates []templateTestResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &templates); err != nil {
		t.Fatalf("decode template list: %v (body=%s)", err, rr.Body.String())
	}
	return templates
}

func templatesBySlug(templates []templateTestResponse) map[string]*templateTestResponse {
	out := make(map[string]*templateTestResponse, len(templates))
	for i := range templates {
		template := templates[i]
		out[template.Slug] = &template
	}
	return out
}
