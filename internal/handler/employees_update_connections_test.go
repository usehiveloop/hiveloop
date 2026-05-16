package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestIntegration_EmployeeUpdate_AttachesOrgConnectionAndMappedSkill(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	category := "engineering"
	if err := h.db.Model(&agent).Update("category", category).Error; err != nil {
		t.Fatalf("set category: %v", err)
	}
	gitGithub := h.seedGlobalSkill(t, "git-github", model.SkillStatusPublished)
	assetUploads := h.seedGlobalSkill(t, "asset-uploads", model.SkillStatusPublished)
	conn := h.seedEmployeeConnection(t, m, "github-app")

	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"connection_ids": []string{conn.ID.String()},
		"skill_ids":      []string{},
	}, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var reloaded model.Agent
	if err := h.db.Where("id = ?", agent.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload agent: %v", err)
	}
	cfg, ok := reloaded.Integrations[conn.ID.String()].(map[string]any)
	if !ok {
		t.Fatalf("connection integration missing: %#v", reloaded.Integrations)
	}
	if actions, ok := cfg["actions"].([]any); !ok || len(actions) != 0 {
		t.Fatalf("actions = %#v, want empty actions", cfg["actions"])
	}
	links := skillIDsFor(t, h.db, agent.ID)
	if !links[gitGithub.ID] {
		t.Fatalf("employee missing mapped git-github skill")
	}
	if !links[assetUploads.ID] {
		t.Fatalf("employee missing default asset-uploads skill")
	}

	var resp struct {
		Employee struct {
			AttachedSkills []struct {
				Name     string `json:"name"`
				Locked   bool   `json:"locked"`
				Required bool   `json:"required"`
			} `json:"attached_skills"`
		} `json:"employee"`
		SyncStatus string `json:"sync_status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SyncStatus != "pending_profile" {
		t.Fatalf("sync_status = %q, want pending_profile", resp.SyncStatus)
	}
	for _, skill := range resp.Employee.AttachedSkills {
		if (skill.Name == "git-github" || skill.Name == "asset-uploads") && (!skill.Locked || !skill.Required) {
			t.Fatalf("skill %s lock flags = locked:%v required:%v, want true", skill.Name, skill.Locked, skill.Required)
		}
	}
}

func TestIntegration_EmployeeUpdate_AttachesBugsinkConnectionSkill(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	bugsink := h.seedGlobalSkill(t, "bugsink", model.SkillStatusPublished)
	conn := h.seedEmployeeConnection(t, m, "bugsink")

	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"connection_ids": []string{conn.ID.String()},
		"skill_ids":      []string{},
	}, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	links := skillIDsFor(t, h.db, agent.ID)
	if !links[bugsink.ID] {
		t.Fatalf("employee missing mapped bugsink skill: %v", links)
	}
}

func TestIntegration_EmployeeAvailableConnections_ExcludesProfileConnections(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	_ = h.seedEmployeeConnection(t, m, "github")
	_ = h.seedEmployeeConnection(t, m, "slack")
	bugsink := h.seedEmployeeConnection(t, m, "bugsink")
	githubApp := h.seedEmployeeConnection(t, m, "github-app")

	rr := h.getEmployeeAvailableConnections(t, m, agent.ID, "member")
	if rr.Code != http.StatusOK {
		t.Fatalf("available connections status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []struct {
			ID       string `json:"id"`
			Provider string `json:"provider"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	seen := map[string]string{}
	for _, conn := range resp.Data {
		seen[conn.Provider] = conn.ID
	}
	if seen["github"] != "" || seen["slack"] != "" {
		t.Fatalf("profile providers were returned: %#v", seen)
	}
	if seen["bugsink"] != bugsink.ID.String() {
		t.Fatalf("bugsink missing from available connections: %#v", seen)
	}
	if seen["github-app"] != githubApp.ID.String() {
		t.Fatalf("github-app missing from available connections: %#v", seen)
	}
}

func TestIntegration_EmployeeUpdate_RejectsProfileConnection(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	conn := h.seedEmployeeConnection(t, m, "github")

	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"connection_ids": []string{conn.ID.String()},
	}, "admin")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeeUpdate_RejectsCrossOrgConnection(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	other := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	conn := h.seedEmployeeConnection(t, other, "github")

	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"connection_ids": []string{conn.ID.String()},
	}, "admin")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeeUpdate_RejectsRevokedConnection(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	conn := h.seedEmployeeConnection(t, m, "github")
	now := time.Now()
	if err := h.db.Model(&conn).Update("revoked_at", &now).Error; err != nil {
		t.Fatalf("revoke connection: %v", err)
	}

	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"connection_ids": []string{conn.ID.String()},
	}, "admin")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}
