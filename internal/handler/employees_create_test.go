package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestIntegration_EmployeesCreate_Engineering_OpenRouter_HappyPath(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	or := h.seedSystemCred(t, "openrouter", false)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	resp := decodeEmployeeResp(t, rr)
	if resp["agent_id"] == "" {
		t.Fatalf("response missing agent id: %v", resp)
	}
	if resp["sandbox_id"] != "" {
		t.Errorf("sandbox_id = %q, want empty until Slack profile is configured", resp["sandbox_id"])
	}
	if resp["status"] != "pending_profile" {
		t.Errorf("status = %q, want pending_profile", resp["status"])
	}

	var agent model.Agent
	if err := h.db.Where("id = ?", resp["agent_id"]).First(&agent).Error; err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if !agent.IsEmployee {
		t.Errorf("agent.is_employee = false, want true")
	}
	if agent.Harness != "employee-sandbox" {
		t.Errorf("agent.harness = %q, want employee-sandbox", agent.Harness)
	}
	if agent.Model != "deepseek/deepseek-v4-flash" {
		t.Errorf("agent.model = %q, want deepseek/deepseek-v4-flash", agent.Model)
	}
	if agent.CredentialID == nil || *agent.CredentialID != or.ID {
		t.Errorf("agent.credential_id = %v, want %v (openrouter)", agent.CredentialID, or.ID)
	}
	if agent.SystemPrompt != "" {
		t.Errorf("agent.system_prompt = %q, want empty for employee bridge v2", agent.SystemPrompt)
	}
	if !strings.Contains(agent.IdentityPrompt, "engineering coordinator employee embedded") {
		t.Errorf("agent.identity_prompt should be set to engineering identity prompt")
	}
	if agent.Status != "active" {
		t.Errorf("agent.status = %q, want active", agent.Status)
	}
	if agent.Category == nil || *agent.Category != "engineering" {
		t.Errorf("agent.category = %v, want engineering", agent.Category)
	}

	var sandboxCount int64
	h.db.Model(&model.Sandbox{}).Where("agent_id = ?", agent.ID).Count(&sandboxCount)
	if sandboxCount != 0 {
		t.Errorf("sandbox rows after create = %d, want 0 until onboarding completion", sandboxCount)
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
	if agent.Model != "deepseek/deepseek-v4-flash" {
		t.Errorf("agent.model = %q, want deepseek/deepseek-v4-flash", agent.Model)
	}
	if agent.CredentialID == nil || *agent.CredentialID != or.ID {
		t.Errorf("agent.credential_id mismatch: got %v want %v", agent.CredentialID, or.ID)
	}
}

func TestIntegration_EmployeesCreate_PrefersOpenRouterWhenBothPresent(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	or := h.seedSystemCred(t, "openrouter", false)
	h.seedSystemCred(t, "crof", false)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	resp := decodeEmployeeResp(t, rr)

	var agent model.Agent
	h.db.Where("id = ?", resp["agent_id"]).First(&agent)
	if agent.Model != "deepseek/deepseek-v4-flash" {
		t.Errorf("openrouter should win: agent.model = %q", agent.Model)
	}
	if agent.CredentialID == nil || *agent.CredentialID != or.ID {
		t.Errorf("openrouter should win: agent.credential_id = %v", agent.CredentialID)
	}
}

func TestIntegration_EmployeesCreate_SkipsRevokedOpenRouter(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", true)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesCreate_NonAdmin_403(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrgWithRole(t, "member")
	h.seedSystemCred(t, "openrouter", false)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rr.Code, rr.Body.String())
	}

	var count int64
	h.db.Model(&model.Agent{}).Where("org_id = ?", org.org.ID).Count(&count)
	if count != 0 {
		t.Errorf("agent rows after 403 = %d, want 0", count)
	}
}

func TestIntegration_EmployeesCreate_InvalidAvatarURL_400(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", false)

	for _, bad := range []string{"javascript:alert(1)", "ftp://example/x", "not-a-url", "/relative/path"} {
		body := validEmployeeBody()
		body["avatar_url"] = bad
		rr := h.post(t, org, body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("avatar_url=%q: status = %d, want 400", bad, rr.Code)
		}
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
	h.seedSystemCred(t, "openrouter", false)

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
	h.seedSystemCred(t, "openrouter", false)

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
	h.seedSystemCred(t, "openrouter", false)

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

func TestIntegration_EmployeesCreate_FirstEmployeeAutoCreatesEngineeringTeam(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", false)

	var teamCount int64
	h.db.Model(&model.Team{}).Where("org_id = ?", org.org.ID).Count(&teamCount)
	if teamCount != 0 {
		t.Fatalf("precondition: org should start with 0 teams, has %d", teamCount)
	}

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	resp := decodeEmployeeResp(t, rr)

	var team model.Team
	if err := h.db.Where("org_id = ? AND name = ?", org.org.ID, "Engineering").First(&team).Error; err != nil {
		t.Fatalf("Engineering team not created: %v", err)
	}

	var agent model.Agent
	h.db.Where("id = ?", resp["agent_id"]).First(&agent)
	if agent.TeamID == nil {
		t.Fatal("agent.team_id is nil, want it set to the Engineering team")
	}
	if *agent.TeamID != team.ID {
		t.Errorf("agent.team_id = %v, want %v (Engineering)", *agent.TeamID, team.ID)
	}
}

func TestIntegration_EmployeesCreate_SecondEmployeeReusesEngineeringTeam(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", false)

	first := h.post(t, org, validEmployeeBody())
	if first.Code != http.StatusCreated {
		t.Fatalf("first create: %d %s", first.Code, first.Body.String())
	}
	second := h.post(t, org, validEmployeeBody())
	if second.Code != http.StatusCreated {
		t.Fatalf("second create: %d %s", second.Code, second.Body.String())
	}

	var teamCount int64
	h.db.Model(&model.Team{}).Where("org_id = ? AND deleted_at IS NULL", org.org.ID).Count(&teamCount)
	if teamCount != 1 {
		t.Fatalf("expected exactly 1 Engineering team, got %d", teamCount)
	}

	firstID := decodeEmployeeResp(t, first)["agent_id"]
	secondID := decodeEmployeeResp(t, second)["agent_id"]
	var firstAgent, secondAgent model.Agent
	h.db.Where("id = ?", firstID).First(&firstAgent)
	h.db.Where("id = ?", secondID).First(&secondAgent)
	if firstAgent.TeamID == nil || secondAgent.TeamID == nil {
		t.Fatalf("both agents should have team_id set; got first=%v second=%v", firstAgent.TeamID, secondAgent.TeamID)
	}
	if *firstAgent.TeamID != *secondAgent.TeamID {
		t.Errorf("expected both employees on same team, got %v vs %v", *firstAgent.TeamID, *secondAgent.TeamID)
	}
}

func TestIntegration_EmployeesCreate_ReusesPreExistingEngineeringTeam(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", false)

	preExisting := model.Team{
		OrgID:       org.org.ID,
		Name:        "Engineering",
		Description: "manually created before any employees",
	}
	if err := h.db.Create(&preExisting).Error; err != nil {
		t.Fatalf("seed pre-existing team: %v", err)
	}

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}

	var teamCount int64
	h.db.Model(&model.Team{}).Where("org_id = ? AND deleted_at IS NULL", org.org.ID).Count(&teamCount)
	if teamCount != 1 {
		t.Fatalf("expected exactly 1 team (the pre-existing one), got %d", teamCount)
	}

	var agent model.Agent
	h.db.Where("id = ?", decodeEmployeeResp(t, rr)["agent_id"]).First(&agent)
	if agent.TeamID == nil || *agent.TeamID != preExisting.ID {
		t.Errorf("agent should have been linked to the pre-existing team %v, got %v", preExisting.ID, agent.TeamID)
	}
	if preExisting.Description == "" {
		t.Fatal("precondition failed")
	}
	var reloaded model.Team
	h.db.Where("id = ?", preExisting.ID).First(&reloaded)
	if reloaded.Description != preExisting.Description {
		t.Errorf("description was overwritten: got %q want %q", reloaded.Description, preExisting.Description)
	}
}

func TestIntegration_EmployeesCreate_TeamsScopedPerOrg(t *testing.T) {
	h := newEmployeeHarness(t)
	orgA := h.createOrg(t)
	orgB := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", false)

	rrA := h.post(t, orgA, validEmployeeBody())
	if rrA.Code != http.StatusCreated {
		t.Fatalf("orgA create: %d", rrA.Code)
	}
	rrB := h.post(t, orgB, validEmployeeBody())
	if rrB.Code != http.StatusCreated {
		t.Fatalf("orgB create: %d", rrB.Code)
	}

	var teamA, teamB model.Team
	if err := h.db.Where("org_id = ? AND name = ?", orgA.org.ID, "Engineering").First(&teamA).Error; err != nil {
		t.Fatalf("orgA team: %v", err)
	}
	if err := h.db.Where("org_id = ? AND name = ?", orgB.org.ID, "Engineering").First(&teamB).Error; err != nil {
		t.Fatalf("orgB team: %v", err)
	}
	if teamA.ID == teamB.ID {
		t.Fatal("each org should get its own Engineering team — got the same id")
	}
}

func TestIntegration_EmployeesCreate_AttachesDefaultSkills(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", false)
	gitGithub := h.seedGlobalSkill(t, "git-github", model.SkillStatusPublished)
	empUploads := h.seedGlobalSkill(t, "employee-public-assets-uploads", model.SkillStatusPublished)
	agentBrowser := h.seedGlobalSkill(t, "agent-browser", model.SkillStatusPublished)
	subUploads := h.seedGlobalSkill(t, "public-assets-uploads", model.SkillStatusPublished)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	agentID := uuid.MustParse(decodeEmployeeResp(t, rr)["agent_id"])

	var link model.AgentSubagent
	if err := h.db.Where("agent_id = ?", agentID).First(&link).Error; err != nil {
		t.Fatalf("load AgentSubagent link: %v", err)
	}
	subagentID := link.SubagentID

	empLinks := skillIDsFor(t, h.db, agentID)
	if !empLinks[gitGithub.ID] || !empLinks[empUploads.ID] || len(empLinks) != 2 {
		t.Errorf("employee skills = %v, want exactly {git-github, employee-public-assets-uploads}", empLinks)
	}

	subLinks := skillIDsFor(t, h.db, subagentID)
	wantSub := []uuid.UUID{agentBrowser.ID, gitGithub.ID, subUploads.ID}
	for _, id := range wantSub {
		if !subLinks[id] {
			t.Errorf("subagent missing skill %v", id)
		}
	}
	if len(subLinks) != 3 {
		t.Errorf("subagent skills count = %d, want 3 (got %v)", len(subLinks), subLinks)
	}

	wantInstallCount := map[uuid.UUID]int{
		gitGithub.ID:    2,
		empUploads.ID:   1,
		agentBrowser.ID: 1,
		subUploads.ID:   1,
	}
	for skillID, want := range wantInstallCount {
		var reloaded model.Skill
		h.db.Where("id = ?", skillID).First(&reloaded)
		if reloaded.InstallCount != want {
			t.Errorf("install_count for %s = %d, want %d", reloaded.Name, reloaded.InstallCount, want)
		}
	}
}

func skillIDsFor(t *testing.T, db *gorm.DB, agentID uuid.UUID) map[uuid.UUID]bool {
	t.Helper()
	var rows []model.AgentSkill
	if err := db.Where("agent_id = ?", agentID).Find(&rows).Error; err != nil {
		t.Fatalf("load agent_skills for %v: %v", agentID, err)
	}
	out := make(map[uuid.UUID]bool, len(rows))
	for _, r := range rows {
		out[r.SkillID] = true
	}
	return out
}

func TestIntegration_EmployeesCreate_CreatesSubagent(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", false)

	body := validEmployeeBody()
	body["name"] = "Alice Engineer"
	rr := h.post(t, org, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	agentID := uuid.MustParse(decodeEmployeeResp(t, rr)["agent_id"])

	var link model.AgentSubagent
	if err := h.db.Where("agent_id = ?", agentID).First(&link).Error; err != nil {
		t.Fatalf("AgentSubagent link not created: %v", err)
	}

	var sub model.Agent
	if err := h.db.Where("id = ?", link.SubagentID).First(&sub).Error; err != nil {
		t.Fatalf("subagent agent not created: %v", err)
	}
	if sub.IsEmployee {
		t.Errorf("subagent.is_employee = true, want false (subagents must not be employees)")
	}
	if sub.Name != "alice-engineer-business-research-specialist" {
		t.Errorf("subagent.name = %q, want %q", sub.Name, "alice-engineer-business-research-specialist")
	}
	if !strings.Contains(sub.SystemPrompt, "Business Research Specialist") {
		t.Errorf("subagent.system_prompt should identify the business research specialist")
	}
	if !strings.Contains(sub.SystemPrompt, "research/{task_id}/report.md") {
		t.Errorf("subagent.system_prompt should contain the employee asset report contract")
	}
	for _, want := range []string{
		"Do not ask clarifying questions",
		"Use todo tools at the start and throughout the task",
		"Sequential research workflow",
		"Use as many parallel agents as needed",
		"search_knowledge_base",
		"Inspect available repositories/codebases",
		"Evidence ledger JSON shape",
		"Contradiction and freshness pass",
	} {
		if !strings.Contains(sub.SystemPrompt, want) {
			t.Errorf("subagent.system_prompt missing %q", want)
		}
	}
	if sub.AgentConfig["default_cloud_agent_type"] != "business_research_specialist" {
		t.Errorf("subagent.agent_config = %#v, want business research specialist marker", sub.AgentConfig)
	}
	if sub.OrgID == nil || *sub.OrgID != org.org.ID {
		t.Errorf("subagent.org_id mismatch")
	}
	if sub.Status != "active" {
		t.Errorf("subagent.status = %q, want active", sub.Status)
	}
}

func TestIntegration_EmployeesCreate_SubagentSlug_AutoIncrementsOnCollision(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	cred := h.seedSystemCred(t, "openrouter", false)

	taken := model.Agent{
		OrgID:        &org.org.ID,
		Name:         "alice-business-research-specialist",
		SystemPrompt: "x",
		Model:        "deepseek/deepseek-v4-flash",
		CredentialID: &cred.ID,
		Status:       "active",
	}
	if err := h.db.Create(&taken).Error; err != nil {
		t.Fatalf("seed colliding agent: %v", err)
	}

	body := validEmployeeBody()
	body["name"] = "Alice"
	rr := h.post(t, org, body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	agentID := uuid.MustParse(decodeEmployeeResp(t, rr)["agent_id"])

	var link model.AgentSubagent
	h.db.Where("agent_id = ?", agentID).First(&link)
	var sub model.Agent
	h.db.Where("id = ?", link.SubagentID).First(&sub)
	if sub.Name != "alice-business-research-specialist-2" {
		t.Errorf("subagent.name = %q, want %q (auto-incremented suffix)", sub.Name, "alice-business-research-specialist-2")
	}
}

func TestIntegration_EmployeesCreate_SubagentUsesOpenRouterDeepSeek(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	or := h.seedSystemCred(t, "openrouter", false)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	agentID := uuid.MustParse(decodeEmployeeResp(t, rr)["agent_id"])

	var link model.AgentSubagent
	h.db.Where("agent_id = ?", agentID).First(&link)
	var sub model.Agent
	h.db.Where("id = ?", link.SubagentID).First(&sub)
	if sub.Model != "deepseek/deepseek-v4-flash" {
		t.Errorf("subagent.model = %q, want deepseek/deepseek-v4-flash", sub.Model)
	}
	if sub.CredentialID == nil || *sub.CredentialID != or.ID {
		t.Errorf("subagent.credential_id = %v, want %v (openrouter)", sub.CredentialID, or.ID)
	}
}

func TestIntegration_EmployeesCreate_SubagentNoCredentialWhenOpenRouterMissing(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesCreate_BestEffort_SkipsMissingDefaultSkill(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", false)
	gitGithub := h.seedGlobalSkill(t, "git-github", model.SkillStatusPublished)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (must succeed despite missing skill): %s", rr.Code, rr.Body.String())
	}
	agentID := decodeEmployeeResp(t, rr)["agent_id"]

	var links []model.AgentSkill
	h.db.Where("agent_id = ?", agentID).Find(&links)
	if len(links) != 1 || links[0].SkillID != gitGithub.ID {
		t.Fatalf("expected only git-github attached, got %d links: %+v", len(links), links)
	}
}

func TestIntegration_EmployeesCreate_IgnoresOrgScopedSkillWithSameName(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", false)

	orgScoped := model.Skill{
		OrgID:      &org.org.ID,
		Slug:       "git-github-org-" + uuid.NewString()[:8],
		Name:       "git-github",
		SourceType: model.SkillSourceInline,
		Status:     model.SkillStatusPublished,
	}
	if err := h.db.Create(&orgScoped).Error; err != nil {
		t.Fatalf("seed org-scoped skill: %v", err)
	}
	t.Cleanup(func() { h.db.Unscoped().Delete(&orgScoped) })

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	agentID := decodeEmployeeResp(t, rr)["agent_id"]

	var count int64
	h.db.Model(&model.AgentSkill{}).Where("agent_id = ?", agentID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 attached skills (org-scoped should not match), got %d", count)
	}
}

func TestIntegration_EmployeesCreate_IgnoresUnpublishedGlobalSkill(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", false)
	h.seedGlobalSkill(t, "git-github", model.SkillStatusDraft)

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	agentID := decodeEmployeeResp(t, rr)["agent_id"]

	var count int64
	h.db.Model(&model.AgentSkill{}).Where("agent_id = ?", agentID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 attached skills (draft should not match), got %d", count)
	}
}

func TestIntegration_EmployeesCreate_DoesNotProvisionSandbox(t *testing.T) {
	h := newEmployeeHarness(t)
	org := h.createOrg(t)
	h.seedSystemCred(t, "openrouter", false)
	h.provider.failOnCreate = true

	rr := h.post(t, org, validEmployeeBody())
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}

	var agentCount, sbCount int64
	h.db.Model(&model.Agent{}).Where("org_id = ?", org.org.ID).Count(&agentCount)
	h.db.Model(&model.Sandbox{}).Where("org_id = ?", org.org.ID).Count(&sbCount)
	if agentCount != 2 {
		t.Errorf("agent rows after create = %d, want employee plus default subagent", agentCount)
	}
	if sbCount != 0 {
		t.Errorf("sandbox rows after create = %d, want 0 until sync/onboarding", sbCount)
	}
	if h.provider.createdCount != 0 {
		t.Errorf("provider create calls = %d, want 0 during employee create", h.provider.createdCount)
	}
}
