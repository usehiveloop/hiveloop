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
	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/middleware"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/tasks"
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
	envCalls, envBearer := h.sidecar.snapshotRuntime()
	if envCalls != 1 {
		t.Errorf("runtime env calls = %d, want 1", envCalls)
	}
	if envBearer == "" || envBearer == "Bearer " {
		t.Errorf("runtime env bearer header missing: %q", envBearer)
	}
	assertEmployeeRuntimeProxyEnv(t, h.sidecar.envBody(), agent.ID)
	h.enqueuer.AssertEnqueued(t, tasks.TypeEmployeeProxyTokenRefresh)
}

func TestIntegration_EmployeesSync_EnvOnlyUpdateCallsRuntimeEnvEndpoint(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("initial sync status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	baseCalls, _ := h.sidecar.snapshot()
	baseEnvCalls, _ := h.sidecar.snapshotRuntime()

	h.setRuntimeEnvVars(t, agent.ID, map[string]string{
		"RUNTIME_TEST_TOKEN": "overlay-value",
	})

	rr = h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("runtime env sync status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Applied          int  `json:"applied"`
		RestartTriggered bool `json:"restart_triggered"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	if resp.Applied < 5 {
		t.Errorf("applied = %d, want at least encrypted env plus proxy env", resp.Applied)
	}
	if resp.RestartTriggered {
		t.Errorf("restart_triggered = %v, want false for env-only sync", resp.RestartTriggered)
	}

	calls, _ := h.sidecar.snapshot()
	if calls != baseCalls {
		t.Errorf("/config calls = %d, want %d", calls, baseCalls)
	}
	envCalls, envBearer := h.sidecar.snapshotRuntime()
	if envCalls != baseEnvCalls+1 {
		t.Errorf("runtime env calls = %d, want %d", envCalls, baseEnvCalls+1)
	}
	if envBearer == "" || envBearer == "Bearer " {
		t.Errorf("runtime env bearer header missing: %q", envBearer)
	}

	var payload map[string]string
	if err := json.Unmarshal(h.sidecar.envBody(), &payload); err != nil {
		t.Fatalf("decode runtime env payload: %v", err)
	}
	if got := payload["RUNTIME_TEST_TOKEN"]; got != "overlay-value" {
		t.Errorf("runtime env payload = %q, want overlay-value", got)
	}
	assertRuntimeEnvProxyPayload(t, payload, agent.ID)
}

func TestIntegration_EmployeesSync_EnvOnlyUpdateRejectsBridgeReservedKeys(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("initial sync status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	h.setRuntimeEnvVars(t, agent.ID, map[string]string{
		"RUNTIME_API_TOKEN": "overlay-value",
		"BRIDGE_INTERNAL":   "forbidden",
	})

	rr = h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("runtime env sync status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var payload map[string]string
	if err := json.Unmarshal(h.sidecar.envBody(), &payload); err != nil {
		t.Fatalf("decode runtime env payload: %v", err)
	}
	if got := payload["RUNTIME_API_TOKEN"]; got != "overlay-value" {
		t.Errorf("RUNTIME_API_TOKEN = %q, want overlay-value", got)
	}
	if _, ok := payload["BRIDGE_INTERNAL"]; ok {
		t.Errorf("BRIDGE_INTERNAL leaked into runtime env payload")
	}
	assertRuntimeEnvProxyPayload(t, payload, agent.ID)
}

func TestIntegration_EmployeesSync_EnvOnlyUpdateCanClearRuntimeEnvOverlay(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("initial sync status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	h.setRuntimeEnvVars(t, agent.ID, map[string]string{
		"CLEARABLE_ENV": "value",
	})
	rr = h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("runtime env sync status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	h.setRuntimeEnvVars(t, agent.ID, map[string]string{})
	rr = h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("runtime env clear status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if rr.Body == nil {
		t.Fatalf("empty sync response body")
	}
	var resp struct {
		Applied int `json:"applied"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	if resp.Applied != 5 {
		t.Errorf("applied = %d, want 5 proxy env vars", resp.Applied)
	}
	var payload map[string]string
	if err := json.Unmarshal(h.sidecar.envBody(), &payload); err != nil {
		t.Fatalf("decode runtime env payload: %v", err)
	}
	if _, ok := payload["CLEARABLE_ENV"]; ok {
		t.Errorf("CLEARABLE_ENV still present in runtime env payload")
	}
	assertRuntimeEnvProxyPayload(t, payload, agent.ID)
}

func TestIntegration_EmployeesSync_DefinitionChangeStillCallsConfigPath(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("initial sync status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	baseConfigCalls, _ := h.sidecar.snapshot()
	updatedDescription := "updated runtime definition"
	if err := h.db.Model(&model.Agent{}).
		Where("id = ?", agent.ID).
		Update("description", updatedDescription).Error; err != nil {
		t.Fatalf("update description: %v", err)
	}

	rr = h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("sync-with-definition-change status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	calls, _ := h.sidecar.snapshot()
	if calls != baseConfigCalls+1 {
		t.Fatalf("runtime /config calls = %d, want %d", calls, baseConfigCalls+1)
	}
	if envCalls, _ := h.sidecar.snapshotRuntime(); envCalls != 2 {
		t.Fatalf("runtime env calls = %d, want 2 after definition change sync", envCalls)
	}
}

func TestIntegration_EmployeesSync_MarksDraftEmployeeActiveAfterRuntimeReady(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	if err := h.db.Model(&model.Agent{}).Where("id = ?", agent.ID).Update("status", "draft").Error; err != nil {
		t.Fatalf("mark draft: %v", err)
	}
	h.seedSandbox(t, m, agent.ID)
	h.seedSlackProfile(t, m, agent.ID)

	rr := h.postSync(t, m, agent.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var stored model.Agent
	if err := h.db.Where("id = ?", agent.ID).First(&stored).Error; err != nil {
		t.Fatalf("load employee: %v", err)
	}
	if stored.Status != "active" {
		t.Fatalf("employee status = %q, want active", stored.Status)
	}
}

func TestIntegration_EmployeesSync_EnsuresDefaultCloudAgents(t *testing.T) {
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
	if len(links) != 2 {
		t.Fatalf("subagent link count = %d, want exactly 2", len(links))
	}
	subagents := defaultSubagentsByType(t, h.db, agent.ID)
	research, ok := subagents["business_research_specialist"]
	if !ok {
		t.Fatalf("business research specialist not ensured: %#v", subagents)
	}
	software, ok := subagents["software_engineering_specialist"]
	if !ok {
		t.Fatalf("software engineering specialist not ensured: %#v", subagents)
	}
	if !strings.Contains(research.SystemPrompt, "Business Research Specialist") {
		t.Fatalf("subagent prompt missing Business Research Specialist identity")
	}
	if !strings.Contains(software.SystemPrompt, "Software Engineering Specialist") {
		t.Fatalf("subagent prompt missing Software Engineering Specialist identity")
	}
	for typ, sub := range subagents {
		if sub.Harness != "open_code" {
			t.Fatalf("%s.harness = %q, want open_code", typ, sub.Harness)
		}
	}
}

func assertEmployeeRuntimeConfig(t *testing.T, body []byte) {
	t.Helper()
	if len(body) == 0 {
		t.Fatal("expected runtime /config request body")
	}
	raw := string(body)
	for _, forbidden := range employeeruntime.EmployeeForbiddenRawProviderEnvKeys() {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("runtime config leaked raw provider key %s", forbidden)
		}
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
	if config.Model.APIKeyEnv != employeeruntime.ProxyAPIKeyEnv {
		t.Errorf("model.api_key_env = %q, want %s", config.Model.APIKeyEnv, employeeruntime.ProxyAPIKeyEnv)
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
	if config.MultimodalModel.APIKeyEnv != employeeruntime.ProxyAPIKeyEnv {
		t.Errorf("multimodal_model.api_key_env = %q, want %s", config.MultimodalModel.APIKeyEnv, employeeruntime.ProxyAPIKeyEnv)
	}
}

func assertEmployeeRuntimeProxyEnv(t *testing.T, body []byte, agentID uuid.UUID) {
	t.Helper()
	var payload map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode runtime env payload: %v", err)
	}
	assertRuntimeEnvProxyPayload(t, payload, agentID)
}

func assertRuntimeEnvProxyPayload(t *testing.T, payload map[string]string, agentID uuid.UUID) {
	t.Helper()
	wantLinearURL := "https://cp.hiveloop.test/internal/linear-proxy/" + agentID.String()
	if got := payload[employeeruntime.EmployeeEnvLinearURL]; got != wantLinearURL {
		t.Errorf("LINEAR_URL = %q, want %q", got, wantLinearURL)
	}
	if got := payload[employeeruntime.EmployeeEnvLinearToken]; got == "" {
		t.Errorf("LINEAR_TOKEN missing")
	}
	wantBugsinkURL := "https://cp.hiveloop.test/internal/bugsink-proxy/" + agentID.String()
	if got := payload[employeeruntime.EmployeeEnvBugsinkURL]; got != wantBugsinkURL {
		t.Errorf("BUGSINK_URL = %q, want %q", got, wantBugsinkURL)
	}
	if _, ok := payload[employeeruntime.EmployeeEnvBugsinkDashboardBaseURL]; !ok {
		t.Errorf("BUGSINK_DASHBOARD_BASE_URL missing")
	}
	if got := payload[employeeruntime.EmployeeEnvBugsinkToken]; got == "" {
		t.Errorf("BUGSINK_TOKEN missing")
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
	h.cfg.SentryDSN = "https://backend@example.com/2"
	h.cfg.EmployeeSandboxSentryDSN = "https://employee@example.com/1"
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
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvRuntimeBindAddr]; got != "0.0.0.0:7080" {
		t.Errorf("RUNTIME_BIND_ADDR = %q, want 0.0.0.0:7080", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.ProxyAPIKeyEnv]; len(got) < 5 || got[:5] != "ptok_" {
		t.Errorf("HIVELOOP_PROXY_API_KEY = %q, want ptok_...", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvAgentModel]; got != employeeruntime.DefaultEmployeeModel {
		t.Errorf("AGENT_MODEL = %q, want deepseek/deepseek-v4-flash", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvAgentBaseURL]; got != "https://proxy.hiveloop.test/v1" {
		t.Errorf("AGENT_BASE_URL = %q, want https://proxy.hiveloop.test/v1", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvAgentAPIKeyEnv]; got != employeeruntime.ProxyAPIKeyEnv {
		t.Errorf("AGENT_API_KEY_ENV = %q, want HIVELOOP_PROXY_API_KEY", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvAgentMultimodalModel]; got != employeeruntime.DefaultEmployeeMultimodalModel {
		t.Errorf("AGENT_MULTIMODAL_MODEL = %q, want google/gemini-3-flash-preview", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvAgentMultimodalBaseURL]; got != "https://proxy.hiveloop.test/v1" {
		t.Errorf("AGENT_MULTIMODAL_BASE_URL = %q, want https://proxy.hiveloop.test/v1", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvAgentMultimodalAPIKeyEnv]; got != employeeruntime.ProxyAPIKeyEnv {
		t.Errorf("AGENT_MULTIMODAL_API_KEY_ENV = %q, want HIVELOOP_PROXY_API_KEY", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvCloudControlPlaneURL]; got != "https://cp.hiveloop.test" {
		t.Errorf("CLOUD_CONTROL_PLANE_URL = %q, want https://cp.hiveloop.test", got)
	}
	wantDriveURL := "https://cp.hiveloop.test/internal/employees/" + agent.ID.String() + "/assets/employee"
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvDriveUploadURL]; got != wantDriveURL {
		t.Errorf("HIVELOOP_DRIVE_UPLOAD_URL = %q, want %q", got, wantDriveURL)
	}
	wantGitCredsURL := "https://cp.hiveloop.test/internal/git-credentials/" + agent.ID.String()
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvGitCredentialsURL]; got != wantGitCredsURL {
		t.Errorf("HIVELOOP_GIT_CREDENTIALS_URL = %q, want %q", got, wantGitCredsURL)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvSentryDSN]; got != "https://employee@example.com/1" {
		t.Errorf("SENTRY_DSN = %q, want employee sandbox DSN", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvSentryEnvironment]; got != "production" {
		t.Errorf("SENTRY_ENVIRONMENT = %q, want production", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvSentryRelease]; got != "employee-bridge@test" {
		t.Errorf("SENTRY_RELEASE = %q, want employee-bridge@test", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvSentrySampleRate]; got != "1" {
		t.Errorf("SENTRY_SAMPLE_RATE = %q, want 1", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvSentryTracesSampleRate]; got != "0.25" {
		t.Errorf("SENTRY_TRACES_SAMPLE_RATE = %q, want 0.25", got)
	}
	if got := h.provider.lastCreateOpts.EnvVars[employeeruntime.EmployeeEnvSentryEnableLogs]; got != "true" {
		t.Errorf("SENTRY_ENABLE_LOGS = %q, want true", got)
	}
	if _, ok := h.provider.lastCreateOpts.EnvVars["SENTRY_DEBUG"]; ok {
		t.Errorf("SENTRY_DEBUG should not be injected into employee sandbox env")
	}
	if _, ok := h.provider.lastCreateOpts.EnvVars["SENTRY_SPOTLIGHT"]; ok {
		t.Errorf("SENTRY_SPOTLIGHT should not be injected into employee sandbox env")
	}
	for _, forbidden := range employeeruntime.EmployeeForbiddenRawProviderEnvKeys() {
		if _, ok := h.provider.lastCreateOpts.EnvVars[forbidden]; ok {
			t.Errorf("%s leaked into employee sandbox env", forbidden)
		}
	}

	var tokenCount int64
	h.db.Model(&model.Token{}).
		Where("meta->>'agent_id' = ? AND meta->>'harness' = ?", agent.ID.String(), "employee-sandbox").
		Count(&tokenCount)
	if tokenCount != 1 {
		t.Errorf("employee-sandbox token rows = %d, want 1", tokenCount)
	}

	var storedSandbox model.Sandbox
	if err := h.db.Where("agent_id = ?", agent.ID).Order("created_at DESC").First(&storedSandbox).Error; err != nil {
		t.Fatalf("load stored employee sandbox: %v", err)
	}
	gotSnapshotID := ""
	if storedSandbox.SnapshotID != nil {
		gotSnapshotID = *storedSandbox.SnapshotID
	}
	if gotSnapshotID != h.cfg.EmployeeSandboxBaseImagePrefix {
		t.Errorf("stored sandbox snapshot_id = %q, want %q", gotSnapshotID, h.cfg.EmployeeSandboxBaseImagePrefix)
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
