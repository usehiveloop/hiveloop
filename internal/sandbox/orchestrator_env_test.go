package sandbox

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/model"
)

func TestEmployeeSandboxEnvVarsUseAPIWebhookBaseURL(t *testing.T) {
	cfg := &config.Config{
		APIWebhookBaseURL: "http://host.docker.internal:8080",
		ProxyHost:         "http://host.docker.internal:8080",
	}
	agent := &model.Employee{ID: uuid.New()}
	sb := &model.Sandbox{ID: uuid.New()}
	env := employeeSandboxEnvVars(cfg, "runtime-secret", sb, uuid.New(), agent, &employeeruntime.StartupSecrets{ProxyToken: "proxy-token"}, nil, "")

	if got := env[employeeruntime.EmployeeEnvCloudControlPlaneURL]; got != "http://host.docker.internal:8080" {
		t.Fatalf("control plane url = %q", got)
	}
	if got := env[employeeruntime.EmployeeEnvAgentBaseURL]; got != "http://host.docker.internal:8080/v1" {
		t.Fatalf("agent base url = %q", got)
	}
	for _, key := range []string{
		employeeruntime.EmployeeEnvGitCredentialsURL,
		employeeruntime.EmployeeEnvDriveUploadURL,
		employeeruntime.EmployeeEnvBugsinkURL,
		employeeruntime.EmployeeEnvLinearURL,
		employeeruntime.EmployeeEnvNotionAPIURL,
	} {
		if got := env[key]; !strings.HasPrefix(got, "http://host.docker.internal:8080/") {
			t.Fatalf("%s = %q", key, got)
		}
	}
}
