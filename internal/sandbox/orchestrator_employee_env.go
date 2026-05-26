package sandbox

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/model"
)

func employeeSandboxEnvVars(cfg *config.Config, runtimeSecret string, sb *model.Sandbox, orgID uuid.UUID, agent *model.Employee, secrets *employeeruntime.StartupSecrets, gitIdentity *employeeGitIdentity, bugsinkDashboardURL string) map[string]string {
	controlPlaneBaseURL := cfg.RuntimeControlPlaneBaseURL()
	proxyBaseURL := cfg.ProxyOpenAIBaseURL()
	envVars := map[string]string{
		employeeruntime.EmployeeEnvRuntimeSecret:            runtimeSecret,
		employeeruntime.ProxyAPIKeyEnv:                      secrets.ProxyToken,
		employeeruntime.EmployeeEnvAgentModel:               employeeruntime.DefaultEmployeeModel,
		employeeruntime.EmployeeEnvAgentBaseURL:             proxyBaseURL,
		employeeruntime.EmployeeEnvAgentAPIKeyEnv:           employeeruntime.ProxyAPIKeyEnv,
		employeeruntime.EmployeeEnvAgentMultimodalModel:     employeeruntime.DefaultEmployeeMultimodalModel,
		employeeruntime.EmployeeEnvAgentMultimodalBaseURL:   proxyBaseURL,
		employeeruntime.EmployeeEnvAgentMultimodalAPIKeyEnv: employeeruntime.ProxyAPIKeyEnv,
		employeeruntime.EmployeeEnvEmployeeID:               agent.ID.String(),
		employeeruntime.EmployeeEnvCloudControlPlaneURL:     controlPlaneBaseURL,
		employeeruntime.EmployeeEnvUploadBearer:             runtimeSecret,
		employeeruntime.EmployeeEnvWorkspaceRoot:            "/workspace",
		employeeruntime.EmployeeEnvDBPath:                   "/app/data/hivy-sandboxes-runtime.db",
		employeeruntime.EmployeeEnvRuntimeBindAddr:          fmt.Sprintf("0.0.0.0:%d", EmployeeSandboxPort),
		employeeruntime.EmployeeEnvRuntimeMode:              "employee",
		employeeruntime.EmployeeEnvSandboxID:                sb.ID.String(),
		employeeruntime.EmployeeEnvOrgID:                    orgID.String(),
		employeeruntime.EmployeeEnvGitUsername:              employeeGitUsername(agent, gitIdentity),
		employeeruntime.EmployeeEnvGitEmail:                 employeeGitEmail(agent, gitIdentity),
		employeeruntime.EmployeeEnvGitCredentialsURL:        fmt.Sprintf("%s/internal/git-credentials/%s", controlPlaneBaseURL, agent.ID),
		employeeruntime.EmployeeEnvGitHubNoKeyring:          "1",
		employeeruntime.EmployeeEnvBugsinkURL:               fmt.Sprintf("%s/internal/bugsink-proxy/%s", controlPlaneBaseURL, agent.ID),
		employeeruntime.EmployeeEnvBugsinkDashboardBaseURL:  bugsinkDashboardURL,
		employeeruntime.EmployeeEnvBugsinkToken:             runtimeSecret,
		employeeruntime.EmployeeEnvLinearURL:                fmt.Sprintf("%s/internal/linear-proxy/%s", controlPlaneBaseURL, agent.ID),
		employeeruntime.EmployeeEnvLinearToken:              runtimeSecret,
		employeeruntime.EmployeeEnvNotionAPIURL:             fmt.Sprintf("%s/internal/notion-proxy/%s", controlPlaneBaseURL, agent.ID),
		employeeruntime.EmployeeEnvNotionToken:              runtimeSecret,
	}
	setEmployeeDriveUploadURL(envVars, cfg, agent.ID, "employee")
	employeeSentryDSN := ""
	if cfg != nil {
		employeeSentryDSN = cfg.SandboxesRuntimeSentryDSN
	}
	setSandboxSentryEnvVars(envVars, cfg, employeeSentryDSN)
	return envVars
}

func buildEmployeeSandboxName(agent *model.Employee) string {
	return fmt.Sprintf("hivy-employee-%s-%s-%d", sanitizeName(agent.Name), shortID(agent.ID), time.Now().Unix())
}

func buildSpecialistRuntimeSandboxName(agent *model.Employee) string {
	return fmt.Sprintf("hivy-specialist-%s-%s-%d", sanitizeName(agent.Name), shortID(agent.ID), time.Now().Unix())
}
