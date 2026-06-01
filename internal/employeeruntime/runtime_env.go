package employeeruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

const (
	runtimeWorkspaceRoot = "/workspace"
	runtimeDBPath        = "/app/data/hivy-sandboxes-runtime.db"
	runtimePort          = 7080
)

func BuildRuntimeEnv(ctx context.Context, deps CompileDeps, agent *model.Employee, sb *model.Sandbox, runtimeSecret string) (map[string]string, error) {
	sandboxID := uuid.Nil
	if sb != nil {
		sandboxID = sb.ID
	}
	token, err := MintProxyToken(ctx, deps, agent, sandboxID)
	if err != nil {
		return nil, err
	}
	return BuildRuntimeEnvWithProxyToken(ctx, deps, agent, sb, runtimeSecret, token)
}

func BuildEmployeeRuntimeConfigUpdate(ctx context.Context, deps CompileDeps, agent *model.Employee, sb *model.Sandbox, runtimeSecret string) (ConfigUpdateRequest, *ProxyTokenResult, error) {
	sandboxID := uuid.Nil
	if sb != nil {
		sandboxID = sb.ID
	}
	token, err := MintProxyToken(ctx, deps, agent, sandboxID)
	if err != nil {
		return ConfigUpdateRequest{}, nil, err
	}
	env, err := BuildRuntimeEnvWithProxyToken(ctx, deps, agent, sb, runtimeSecret, token)
	if err != nil {
		return ConfigUpdateRequest{}, token, err
	}
	def, err := CompileWithProxyToken(ctx, deps, agent, token)
	if err != nil {
		return ConfigUpdateRequest{}, token, err
	}
	def.OutboundChannels = ControlPlaneOutboundChannels(deps.Cfg, sandboxID)
	return ConfigUpdateRequest{
		Definition: def,
		RuntimeEnv: env,
	}, token, nil
}

func BuildRuntimeEnvWithProxyToken(ctx context.Context, deps CompileDeps, agent *model.Employee, sb *model.Sandbox, runtimeSecret string, token *ProxyTokenResult) (map[string]string, error) {
	env := make(map[string]string)
	if agent == nil {
		return env, nil
	}
	if token == nil || token.Token == "" || token.JTI == "" {
		return nil, fmt.Errorf("runtime env proxy token is required")
	}
	if sb != nil {
		env[EmployeeEnvSandboxID] = sb.ID.String()
	}
	env[EmployeeEnvRuntimeSecret] = runtimeSecret
	env[EmployeeEnvUploadBearer] = runtimeSecret
	env[EmployeeEnvEmployeeID] = agent.ID.String()
	if agent.OrgID != nil {
		env[EmployeeEnvOrgID] = agent.OrgID.String()
	}
	if deps.Cfg != nil {
		env[EmployeeEnvCloudControlPlaneURL] = deps.Cfg.RuntimeControlPlaneBaseURL()
		env[EmployeeEnvAgentBaseURL] = deps.Cfg.ProxyOpenAIBaseURL()
		env[EmployeeEnvAgentMultimodalBaseURL] = deps.Cfg.ProxyOpenAIBaseURL()
		env[EmployeeEnvWorkspaceRoot] = runtimeWorkspaceRoot
		env[EmployeeEnvDBPath] = runtimeDBPath
		env[EmployeeEnvRuntimeBindAddr] = fmt.Sprintf("0.0.0.0:%d", runtimePort)
	}
	env[EmployeeEnvAgentModel] = DefaultEmployeeModel
	env[EmployeeEnvAgentAPIKeyEnv] = ProxyAPIKeyEnv
	env[EmployeeEnvAgentMultimodalModel] = DefaultEmployeeMultimodalModel
	env[EmployeeEnvAgentMultimodalAPIKeyEnv] = ProxyAPIKeyEnv
	env[EmployeeEnvRuntimeMode] = "employee"
	env[ProxyAPIKeyEnv] = token.Token

	if err := addAgentRuntimeEnv(ctx, deps, env, agent, runtimeSecret); err != nil {
		return nil, err
	}
	return env, nil
}

func addAgentRuntimeEnv(ctx context.Context, deps CompileDeps, env map[string]string, agent *model.Employee, runtimeSecret string) error {
	if len(agent.EncryptedEnvVars) > 0 {
		if deps.EncKey == nil {
			return fmt.Errorf("runtime env decrypt: encryption key is required")
		}
		decrypted, err := deps.EncKey.DecryptString(agent.EncryptedEnvVars)
		if err != nil {
			return err
		}
		decrypted = strings.TrimSpace(decrypted)
		if decrypted != "" {
			rawEnv := map[string]string{}
			if err := json.Unmarshal([]byte(decrypted), &rawEnv); err != nil {
				return fmt.Errorf("decode env vars: %w", err)
			}
			for key, value := range rawEnv {
				env[key] = value
			}
		}
	}
	addControlPlaneRuntimeEnv(ctx, deps, env, agent, runtimeSecret)
	return nil
}

func addControlPlaneRuntimeEnv(ctx context.Context, deps CompileDeps, env map[string]string, agent *model.Employee, runtimeSecret string) {
	if env == nil || deps.Cfg == nil || agent == nil || agent.ID == uuid.Nil || runtimeSecret == "" {
		return
	}
	controlPlaneBaseURL := deps.Cfg.RuntimeControlPlaneBaseURL()
	env[EmployeeEnvBugsinkURL] = fmt.Sprintf("%s/internal/bugsink-proxy/%s", controlPlaneBaseURL, agent.ID)
	if deps.DB != nil && agent.OrgID != nil {
		env[EmployeeEnvBugsinkDashboardBaseURL] = BugsinkDashboardBaseURL(ctx, deps.DB, *agent.OrgID, *agent)
	}
	env[EmployeeEnvBugsinkToken] = runtimeSecret
	env[EmployeeEnvLinearURL] = fmt.Sprintf("%s/internal/linear-proxy/%s", controlPlaneBaseURL, agent.ID)
	env[EmployeeEnvLinearToken] = runtimeSecret
	env[EmployeeEnvNotionAPIURL] = fmt.Sprintf("%s/internal/notion-proxy/%s", controlPlaneBaseURL, agent.ID)
	env[EmployeeEnvNotionToken] = runtimeSecret
}
