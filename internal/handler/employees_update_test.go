package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/auth"
	"github.com/usehiveloop/hiveloop/internal/handler"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func TestIntegration_EmployeeUpdate_EditsAllowedFieldsAndKeepsCategory(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	category := "engineering"
	if err := h.db.Model(&agent).Updates(map[string]any{
		"category":    category,
		"description": "old description",
		"avatar_url":  "https://cdn.example/old.png",
	}).Error; err != nil {
		t.Fatalf("update category: %v", err)
	}

	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"name":        "Updated employee",
		"description": "Updated description",
		"avatar_url":  "https://cdn.example/new.png",
	}, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var reloaded model.Agent
	if err := h.db.Where("id = ?", agent.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload agent: %v", err)
	}
	if reloaded.Name != "Updated employee" {
		t.Fatalf("name = %q, want Updated employee", reloaded.Name)
	}
	if reloaded.Category == nil || *reloaded.Category != category {
		t.Fatalf("category = %v, want unchanged engineering", reloaded.Category)
	}
}

func TestIntegration_EmployeeUpdate_ModelUpdatesCredential(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	googleCred := h.seedSystemCred(t, "google", false)

	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"model": " gemini-3-flash-preview ",
	}, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var reloaded model.Agent
	if err := h.db.Where("id = ?", agent.ID).First(&reloaded).Error; err != nil {
		t.Fatalf("reload agent: %v", err)
	}
	if reloaded.Model != "gemini-3-flash-preview" {
		t.Fatalf("model = %q, want gemini-3-flash-preview", reloaded.Model)
	}
	if reloaded.CredentialID == nil || *reloaded.CredentialID != googleCred.ID {
		t.Fatalf("credential_id = %v, want %s", reloaded.CredentialID, googleCred.ID)
	}
}

func TestIntegration_EmployeeUpdate_ModelRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		wantError string
	}{
		{name: "empty", model: " ", wantError: "model is required"},
		{name: "unknown", model: "does-not-exist", wantError: "not in the catalog"},
		{name: "hidden", model: "openai/gpt-5-nano", wantError: "not selectable"},
		{name: "unbacked", model: "gpt-5.4", wantError: "not backed by an active system credential"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newEmployeeHarness(t)
			h.platformCredCleanup(t)
			m := h.createOrg(t)
			agent := h.seedEmployeeAgent(t, m)

			rr := h.putEmployee(t, m, agent.ID, map[string]any{
				"model": tt.model,
			}, "admin")
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tt.wantError) {
				t.Fatalf("error = %s, want substring %q", rr.Body.String(), tt.wantError)
			}

			var reloaded model.Agent
			if err := h.db.Where("id = ?", agent.ID).First(&reloaded).Error; err != nil {
				t.Fatalf("reload agent: %v", err)
			}
			if reloaded.Model != agent.Model {
				t.Fatalf("model changed after rejected update: got %q want %q", reloaded.Model, agent.Model)
			}
		})
	}
}

func TestIntegration_EmployeeUpdate_BlockedByActiveUpgrade(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	upgrade := model.EmployeeSandboxUpgrade{
		OrgID:        m.org.ID,
		AgentID:      agent.ID,
		Status:       model.EmployeeSandboxUpgradeStatusRunning,
		Phase:        model.EmployeeSandboxUpgradePhaseBackup,
		OldSandboxID: nil,
	}
	if err := h.db.Create(&upgrade).Error; err != nil {
		t.Fatalf("create upgrade: %v", err)
	}

	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"description": "blocked",
	}, "admin")
	if rr.Code != http.StatusConflict {
		t.Fatalf("update status = %d, want 409: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeeUpdate_PreservesRequiredSkills(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	category := "engineering"
	if err := h.db.Model(&agent).Update("category", category).Error; err != nil {
		t.Fatalf("set category: %v", err)
	}
	gitGithub := h.seedGlobalSkill(t, "git-github", model.SkillStatusPublished)
	assetUploads := h.seedGlobalSkill(t, "asset-uploads", model.SkillStatusPublished)

	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"skill_ids": []string{},
	}, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	links := skillIDsFor(t, h.db, agent.ID)
	if !links[gitGithub.ID] || !links[assetUploads.ID] {
		t.Fatalf("required skills not preserved: %v", links)
	}
}

func TestIntegration_EmployeeSkillDetach_RejectsRequiredSkill(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	category := "engineering"
	if err := h.db.Model(&agent).Update("category", category).Error; err != nil {
		t.Fatalf("set category: %v", err)
	}
	gitGithub := h.seedGlobalSkill(t, "git-github", model.SkillStatusPublished)
	_ = h.seedGlobalSkill(t, "asset-uploads", model.SkillStatusPublished)

	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"skill_ids": []string{},
	}, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	req := httptest.NewRequest(http.MethodDelete, "/v1/agents/"+agent.ID.String()+"/skills/"+gitGithub.ID.String(), nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "admin",
	})

	skillH := handler.NewSkillHandler(h.db, h.enqueuer)
	r := chi.NewRouter()
	r.Route("/v1/agents/{agentID}/skills", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(h.db))
		r.Use(middleware.RequireOrgAdmin(h.db))
		r.Delete("/{skillID}", skillH.DetachFromAgent)
	})

	detachRR := httptest.NewRecorder()
	r.ServeHTTP(detachRR, req)
	if detachRR.Code != http.StatusConflict {
		t.Fatalf("detach status = %d, want 409: %s", detachRR.Code, detachRR.Body.String())
	}
}

func TestIntegration_EmployeeSkillDetach_RejectsConnectionRequiredSkill(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodDelete, "/v1/agents/"+agent.ID.String()+"/skills/"+bugsink.ID.String(), nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "admin",
	})

	skillH := handler.NewSkillHandler(h.db, h.enqueuer)
	r := chi.NewRouter()
	r.Route("/v1/agents/{agentID}/skills", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(h.db))
		r.Use(middleware.RequireOrgAdmin(h.db))
		r.Delete("/{skillID}", skillH.DetachFromAgent)
	})

	detachRR := httptest.NewRecorder()
	r.ServeHTTP(detachRR, req)
	if detachRR.Code != http.StatusConflict {
		t.Fatalf("detach status = %d, want 409: %s", detachRR.Code, detachRR.Body.String())
	}
}

func TestIntegration_EmployeeUpdate_SyncsWhenProfileExists(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)

	beforeCalls, _ := h.sidecar.snapshot()
	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"description": "fresh runtime config",
	}, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	afterCalls, _ := h.sidecar.snapshot()
	if afterCalls != beforeCalls+1 {
		t.Fatalf("sync calls = %d, want %d", afterCalls, beforeCalls+1)
	}
}

func TestIntegration_EmployeeUpdate_ModelSyncsWhenProfileExists(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)

	beforeCalls, _ := h.sidecar.snapshot()
	rr := h.putEmployee(t, m, agent.ID, map[string]any{
		"model": "deepseek/deepseek-v4-pro",
	}, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	afterCalls, _ := h.sidecar.snapshot()
	if afterCalls != beforeCalls+1 {
		t.Fatalf("sync calls = %d, want %d", afterCalls, beforeCalls+1)
	}

	var config struct {
		Model struct {
			ModelID string `json:"model_id"`
		} `json:"model"`
	}
	if err := json.Unmarshal(h.sidecar.configBody(), &config); err != nil {
		t.Fatalf("decode runtime config: %v", err)
	}
	if config.Model.ModelID != "deepseek/deepseek-v4-pro" {
		t.Fatalf("runtime model_id = %q, want deepseek/deepseek-v4-pro", config.Model.ModelID)
	}
}
