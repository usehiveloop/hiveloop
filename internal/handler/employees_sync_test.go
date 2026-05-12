package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if resp["applied"].(float64) != 1 {
		t.Errorf("applied = %v, want 1", resp["applied"])
	}
	if resp["repos_cloned"].(float64) != 0 {
		t.Errorf("repos_cloned = %v, want 0", resp["repos_cloned"])
	}
	if resp["restart_triggered"] != true {
		t.Errorf("restart_triggered = %v, want true", resp["restart_triggered"])
	}

	calls, bearer := h.sidecar.snapshot()
	if calls != 1 {
		t.Errorf("runtime /config called %d times, want 1", calls)
	}
	if bearer == "" || bearer == "Bearer " {
		t.Errorf("sidecar bearer header missing: %q", bearer)
	}
	assertEmployeeRuntimeConfig(t, h.sidecar.configBody())
}

func TestIntegration_EmployeesSync_EnsuresBusinessResearchSpecialist(t *testing.T) {
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
	rr = h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("second sync status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var links []model.AgentSubagent
	if err := h.db.Where("agent_id = ?", agent.ID).Find(&links).Error; err != nil {
		t.Fatalf("load subagent links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("subagent link count = %d, want exactly 1", len(links))
	}
	var sub model.Agent
	if err := h.db.Where("id = ?", links[0].SubagentID).First(&sub).Error; err != nil {
		t.Fatalf("load subagent: %v", err)
	}
	if sub.AgentConfig["default_cloud_agent_type"] != "business_research_specialist" {
		t.Fatalf("subagent agent_config = %#v", sub.AgentConfig)
	}
	if !strings.Contains(sub.SystemPrompt, "Business Research Specialist") {
		t.Fatalf("subagent prompt missing Business Research Specialist identity")
	}
}

func assertEmployeeRuntimeConfig(t *testing.T, body []byte) {
	t.Helper()
	if len(body) == 0 {
		t.Fatal("expected runtime /config request body")
	}
	raw := string(body)
	if strings.Contains(raw, "OPENROUTER_API_KEY") {
		t.Fatal("runtime config leaked OPENROUTER_API_KEY")
	}
	if strings.Contains(raw, "sk-openrouter-test") {
		t.Fatal("runtime config leaked decrypted OpenRouter credential")
	}
	var config struct {
		Model struct {
			Provider  string `json:"provider"`
			BaseURL   string `json:"base_url"`
			ModelID   string `json:"model_id"`
			APIKeyEnv string `json:"api_key_env"`
		} `json:"model"`
		MultimodalModel struct {
			Provider  string `json:"provider"`
			BaseURL   string `json:"base_url"`
			ModelID   string `json:"model_id"`
			APIKeyEnv string `json:"api_key_env"`
		} `json:"multimodal_model"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatalf("decode runtime config: %v", err)
	}
	if config.Model.Provider != "openai_compatible" {
		t.Errorf("model.provider = %q, want openai_compatible", config.Model.Provider)
	}
	if config.Model.ModelID != "deepseek/deepseek-v4-flash" {
		t.Errorf("model.model_id = %q, want deepseek/deepseek-v4-flash", config.Model.ModelID)
	}
	if config.Model.BaseURL != "https://proxy.hiveloop.test/v1" {
		t.Errorf("model.base_url = %q, want https://proxy.hiveloop.test/v1", config.Model.BaseURL)
	}
	if config.Model.APIKeyEnv != "HIVELOOP_PROXY_API_KEY" {
		t.Errorf("model.api_key_env = %q, want HIVELOOP_PROXY_API_KEY", config.Model.APIKeyEnv)
	}
	if config.MultimodalModel.Provider != "openai_compatible" {
		t.Errorf("multimodal_model.provider = %q, want openai_compatible", config.MultimodalModel.Provider)
	}
	if config.MultimodalModel.ModelID != "google/gemini-3-flash-preview" {
		t.Errorf("multimodal_model.model_id = %q, want google/gemini-3-flash-preview", config.MultimodalModel.ModelID)
	}
	if config.MultimodalModel.BaseURL != "https://proxy.hiveloop.test/v1" {
		t.Errorf("multimodal_model.base_url = %q, want https://proxy.hiveloop.test/v1", config.MultimodalModel.BaseURL)
	}
	if config.MultimodalModel.APIKeyEnv != "HIVELOOP_PROXY_API_KEY" {
		t.Errorf("multimodal_model.api_key_env = %q, want HIVELOOP_PROXY_API_KEY", config.MultimodalModel.APIKeyEnv)
	}
}

func TestIntegration_EmployeesSync_WhatsappProfileDoesNotSatisfyRuntimeGate(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedWhatsappProfile(t, m, agent.ID)

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_EmployeesSync_NotEmployee_400(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	cred := h.seedSystemCred(t, "openrouter", false)
	credID := cred.ID
	agent := model.Agent{
		OrgID:        &m.org.ID,
		Name:         "not-employee",
		IsEmployee:   false,
		Harness:      "employee-sandbox",
		Model:        "deepseek/deepseek-v4-flash",
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

func TestIntegration_EmployeesSync_NoSandbox_Provisions(t *testing.T) {
	h := newEmployeeHarness(t)
	h.cfg.Environment = "production"
	h.cfg.SentryDSN = "https://public@example.com/1"
	h.cfg.SentryRelease = "employee-bridge@test"
	h.cfg.SentryTracesSampleRate = 0.25
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSlackProfile(t, m, agent.ID)
	// no sandbox seeded

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	if h.provider.createdCount != 1 {
		t.Fatalf("provider create calls = %d, want 1", h.provider.createdCount)
	}
	if h.provider.lastCreateOpts.SnapshotID != "hiveloop-employee-sandbox-test-small-v1" {
		t.Errorf("snapshot = %q, want employee sandbox snapshot", h.provider.lastCreateOpts.SnapshotID)
	}
	if got := h.provider.lastCreateOpts.EnvVars["RUNTIME_BIND_ADDR"]; got != "0.0.0.0:7080" {
		t.Errorf("RUNTIME_BIND_ADDR = %q, want 0.0.0.0:7080", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars["HIVELOOP_PROXY_API_KEY"]; len(got) < 5 || got[:5] != "ptok_" {
		t.Errorf("HIVELOOP_PROXY_API_KEY = %q, want ptok_...", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars["AGENT_MODEL"]; got != "deepseek/deepseek-v4-flash" {
		t.Errorf("AGENT_MODEL = %q, want deepseek/deepseek-v4-flash", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars["AGENT_BASE_URL"]; got != "https://proxy.hiveloop.test/v1" {
		t.Errorf("AGENT_BASE_URL = %q, want https://proxy.hiveloop.test/v1", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars["AGENT_API_KEY_ENV"]; got != "HIVELOOP_PROXY_API_KEY" {
		t.Errorf("AGENT_API_KEY_ENV = %q, want HIVELOOP_PROXY_API_KEY", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars["AGENT_MULTIMODAL_MODEL"]; got != "google/gemini-3-flash-preview" {
		t.Errorf("AGENT_MULTIMODAL_MODEL = %q, want google/gemini-3-flash-preview", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars["AGENT_MULTIMODAL_API_KEY_ENV"]; got != "HIVELOOP_PROXY_API_KEY" {
		t.Errorf("AGENT_MULTIMODAL_API_KEY_ENV = %q, want HIVELOOP_PROXY_API_KEY", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars["SENTRY_DSN"]; got != "https://public@example.com/1" {
		t.Errorf("SENTRY_DSN = %q, want backend configured DSN", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars["SENTRY_ENVIRONMENT"]; got != "production" {
		t.Errorf("SENTRY_ENVIRONMENT = %q, want production", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars["SENTRY_RELEASE"]; got != "employee-bridge@test" {
		t.Errorf("SENTRY_RELEASE = %q, want employee-bridge@test", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars["SENTRY_SAMPLE_RATE"]; got != "1" {
		t.Errorf("SENTRY_SAMPLE_RATE = %q, want 1", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars["SENTRY_TRACES_SAMPLE_RATE"]; got != "0.25" {
		t.Errorf("SENTRY_TRACES_SAMPLE_RATE = %q, want 0.25", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars["SENTRY_ENABLE_LOGS"]; got != "true" {
		t.Errorf("SENTRY_ENABLE_LOGS = %q, want true", got)
	}
	if _, ok := h.provider.lastCreateOpts.EnvVars["SENTRY_DEBUG"]; ok {
		t.Errorf("SENTRY_DEBUG should not be injected into employee sandbox env")
	}
	if _, ok := h.provider.lastCreateOpts.EnvVars["SENTRY_SPOTLIGHT"]; ok {
		t.Errorf("SENTRY_SPOTLIGHT should not be injected into employee sandbox env")
	}
	if _, ok := h.provider.lastCreateOpts.EnvVars["OPENROUTER_API_KEY"]; ok {
		t.Errorf("OPENROUTER_API_KEY leaked into employee sandbox env")
	}

	var tokenCount int64
	h.db.Model(&model.Token{}).
		Where("meta->>'agent_id' = ? AND meta->>'harness' = ?", agent.ID.String(), "employee-sandbox").
		Count(&tokenCount)
	if tokenCount != 1 {
		t.Errorf("employee-sandbox token rows = %d, want 1", tokenCount)
	}
}

func TestIntegration_EmployeesSync_ErrorSandbox_ProvisionsFreshRuntime(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	old := h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)
	if err := h.db.Model(&old).Updates(map[string]any{
		"status":        "error",
		"error_message": "employee runtime not live",
	}).Error; err != nil {
		t.Fatalf("mark sandbox error: %v", err)
	}

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	if h.provider.createdCount != 1 {
		t.Fatalf("provider create calls = %d, want 1", h.provider.createdCount)
	}
	var fresh model.Sandbox
	if err := h.db.Where("agent_id = ? AND status = ?", agent.ID, "running").
		Order("created_at DESC").First(&fresh).Error; err != nil {
		t.Fatalf("load fresh sandbox: %v", err)
	}
	if fresh.ID == old.ID {
		t.Fatalf("reused failed sandbox %s; want fresh sandbox", old.ID)
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
