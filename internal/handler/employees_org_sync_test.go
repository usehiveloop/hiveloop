package handler_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/model"
)

func TestEmployeeHandlerSyncOrgHivyEmployee_CreatesSandboxAndPushesConfig(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)

	if err := h.handler.SyncOrgHivyEmployee(t.Context(), m.org.ID); err != nil {
		t.Fatalf("SyncOrgHivyEmployee: %v", err)
	}

	configCalls, configBearer := h.sidecar.snapshot()
	if configCalls != 1 {
		t.Fatalf("config sync calls = %d, want 1", configCalls)
	}
	if configBearer == "" {
		t.Fatal("config sync bearer should be set")
	}
	envCalls, envBearer := h.sidecar.snapshotRuntime()
	if envCalls != 1 {
		t.Fatalf("runtime env sync calls = %d, want 1", envCalls)
	}
	if envBearer == "" {
		t.Fatal("runtime env bearer should be set")
	}

	var sb model.Sandbox
	if err := h.db.Where("org_id = ? AND employee_id = ? AND status = ?", m.org.ID, agent.ID, "running").First(&sb).Error; err != nil {
		t.Fatalf("load running sandbox: %v", err)
	}
	if sb.ExternalID == "" || sb.RuntimeURL == "" {
		t.Fatalf("sandbox missing provider fields: %#v", sb)
	}
	assertPushedProxyTokenMatchesMCPURL(t, h.sidecar.configBody(), h.sidecar.envBody())
}

func assertPushedProxyTokenMatchesMCPURL(t *testing.T, configBody, envBody []byte) {
	t.Helper()
	var env map[string]string
	if err := json.Unmarshal(envBody, &env); err != nil {
		t.Fatalf("decode runtime env: %v", err)
	}
	rawToken := strings.TrimPrefix(env[employeeruntime.ProxyAPIKeyEnv], "ptok_")
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		t.Fatalf("proxy token should be JWT-shaped")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode proxy token payload: %v", err)
	}
	var claims struct {
		JTI string `json:"jti"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("decode proxy token claims: %v", err)
	}
	if claims.JTI == "" {
		t.Fatal("proxy token missing jti")
	}

	var def struct {
		McpServers []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"mcp_servers"`
	}
	if err := json.Unmarshal(configBody, &def); err != nil {
		t.Fatalf("decode pushed definition: %v", err)
	}
	var mcpURL string
	for _, srv := range def.McpServers {
		if srv.Name == "hivy" {
			mcpURL = srv.URL
			break
		}
	}
	if mcpURL == "" {
		t.Fatal("pushed definition missing hivy MCP server")
	}
	if !strings.HasSuffix(strings.TrimRight(mcpURL, "/"), "/"+claims.JTI) {
		t.Fatalf("MCP URL %q does not match proxy token jti %q", mcpURL, claims.JTI)
	}
}
