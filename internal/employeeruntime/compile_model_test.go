package employeeruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestControlPlaneOutboundChannels_EmitsEmployeeWebhookSpec(t *testing.T) {
	sandboxID := uuid.New()
	channels := ControlPlaneOutboundChannels(&config.Config{SpecialistSandboxHost: "api.hivy.test"}, sandboxID)
	if len(channels) != 1 {
		t.Fatalf("channels = %#v", channels)
	}
	channel, ok := channels[0].(map[string]any)
	if !ok {
		t.Fatalf("channel has wrong type: %#v", channels[0])
	}
	if channel["url"] != "https://api.hivy.test/internal/webhooks/employee/"+sandboxID.String() {
		t.Fatalf("url = %q", channel["url"])
	}
	if channel["secret_env"] != EmployeeEnvRuntimeSecret {
		t.Fatalf("secret env = %q", channel["secret_env"])
	}
}

func TestControlPlaneOutboundChannels_UsesAPIWebhookBaseURL(t *testing.T) {
	sandboxID := uuid.New()
	channels := ControlPlaneOutboundChannels(&config.Config{APIWebhookBaseURL: "http://host.docker.internal:8080"}, sandboxID)
	channel := channels[0].(map[string]any)
	if channel["url"] != "http://host.docker.internal:8080/internal/webhooks/employee/"+sandboxID.String() {
		t.Fatalf("url = %q", channel["url"])
	}
}

func TestCompile_ReferencesProxyEnvInsteadOfRawProviderKeys(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Employee{ID: uuid.New(), OrgID: &orgID, Name: "Aria", Model: DefaultEmployeeModel}

	def, err := Compile(context.Background(), CompileDeps{Cfg: &config.Config{ProxyHost: "proxy.hivy.test"}}, agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if def.Model.APIKeyEnv != ProxyAPIKeyEnv {
		t.Fatalf("model.api_key_env = %q, want %q", def.Model.APIKeyEnv, ProxyAPIKeyEnv)
	}
	if def.Model.BaseURL != "https://proxy.hivy.test/v1/proxy/v1" {
		t.Fatalf("model.base_url = %q", def.Model.BaseURL)
	}
	if def.MultimodalModel == nil || def.MultimodalModel.APIKeyEnv != ProxyAPIKeyEnv {
		t.Fatalf("multimodal_model.api_key_env = %#v, want %q", def.MultimodalModel, ProxyAPIKeyEnv)
	}
	if employeeMCPAuthorizationHeader() != "Bearer ${"+ProxyAPIKeyEnv+"}" {
		t.Fatalf("MCP auth header references wrong env: %q", employeeMCPAuthorizationHeader())
	}

	bashConfig, ok := def.Tools[0]["config"].(map[string]any)
	if !ok {
		t.Fatalf("bash tool config has wrong type: %#v", def.Tools[0]["config"])
	}
	envPassthrough, ok := bashConfig["env_passthrough"].([]string)
	if !ok {
		t.Fatalf("env_passthrough has wrong type: %#v", bashConfig["env_passthrough"])
	}
	for _, key := range []string{EmployeeEnvHome, EmployeeEnvPath, EmployeeEnvLang, EmployeeEnvLCAll, ProxyAPIKeyEnv, EmployeeEnvBugsinkURL, EmployeeEnvBugsinkDashboardBaseURL, EmployeeEnvBugsinkToken, EmployeeEnvLinearURL, EmployeeEnvLinearToken, EmployeeEnvNotionAPIURL, EmployeeEnvNotionToken} {
		if !containsString(envPassthrough, key) {
			t.Fatalf("env_passthrough missing %s: %#v", key, envPassthrough)
		}
	}

	body, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	for _, forbidden := range EmployeeForbiddenRawProviderEnvKeys() {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("runtime config leaked raw provider key %s: %s", forbidden, string(body))
		}
	}
}

func TestCompile_UsesAgentModelWithDefaultFallback(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Employee{ID: uuid.New(), OrgID: &orgID, Name: "Aria", Model: "deepseek-v4-pro"}

	def, err := Compile(context.Background(), CompileDeps{Cfg: &config.Config{}}, agent)
	if err != nil {
		t.Fatalf("compile custom model: %v", err)
	}
	if def.Model.ModelID != "deepseek-v4-pro" {
		t.Fatalf("model_id = %q, want deepseek-v4-pro", def.Model.ModelID)
	}

	agent.Model = " "
	def, err = Compile(context.Background(), CompileDeps{Cfg: &config.Config{}}, agent)
	if err != nil {
		t.Fatalf("compile blank model: %v", err)
	}
	if def.Model.ModelID != DefaultEmployeeModel {
		t.Fatalf("blank model fallback = %q, want %q", def.Model.ModelID, DefaultEmployeeModel)
	}
}
