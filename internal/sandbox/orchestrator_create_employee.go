package sandbox

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/employeeruntime"
	"github.com/usehiveloop/hiveloop/internal/logging"
	"github.com/usehiveloop/hiveloop/internal/model"
)

const (
	EmployeeSandboxPort    = 7080
	employeeHealthTimeout  = 90 * time.Second
	employeeHealthInterval = 2 * time.Second
)

func (o *Orchestrator) CreateEmployeeSandbox(ctx context.Context, agent *model.Agent, secrets *employeeruntime.StartupSecrets) (*model.Sandbox, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("CreateEmployeeSandbox: agent must have org_id")
	}
	if secrets == nil || secrets.SlackBotToken == "" || secrets.SlackAppToken == "" || secrets.ProxyToken == "" {
		return nil, fmt.Errorf("CreateEmployeeSandbox: slack tokens and proxy token are required")
	}
	orgID := *agent.OrgID

	gitIdentity, err := o.loadAgentGitIdentity(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("loading employee git identity: %w", err)
	}

	runtimeSecret, err := generateRandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generating runtime secret: %w", err)
	}
	encryptedSecret, err := o.encKey.EncryptString(runtimeSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypting runtime secret: %w", err)
	}

	snapshotID := o.cfg.EmployeeSandboxBaseImagePrefix
	sb := model.Sandbox{
		OrgID:                 &orgID,
		AgentID:               &agent.ID,
		SnapshotID:            &snapshotID,
		EncryptedBridgeAPIKey: encryptedSecret,
		Status:                "creating",
	}
	if err := o.db.Create(&sb).Error; err != nil {
		return nil, fmt.Errorf("saving sandbox: %w", err)
	}

	envVars := employeeSandboxEnvVars(o.cfg, runtimeSecret, &sb, orgID, agent, secrets, gitIdentity)
	labels := map[string]string{
		"org_id":     orgID.String(),
		"sandbox_id": sb.ID.String(),
		"agent_id":   agent.ID.String(),
		"harness":    "employee-sandbox",
	}

	info, err := o.provider.CreateSandbox(ctx, CreateSandboxOpts{
		Name:       buildEmployeeSandboxName(agent),
		SnapshotID: snapshotID,
		EnvVars:    envVars,
		Labels:     labels,
	})
	if err != nil {
		if delErr := o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error; delErr != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "delete orphaned employee sandbox row after provider create failure",
				"error", delErr, "sandbox_id", sb.ID)
		}
		return nil, fmt.Errorf("provider create: %w", err)
	}

	sandboxURL, err := o.provider.GetEndpoint(ctx, info.ExternalID, EmployeeSandboxPort)
	if err != nil {
		o.markSandboxError(ctx, &sb, map[string]any{
			"external_id":   info.ExternalID,
			"status":        "error",
			"error_message": "get endpoint failed",
		})
		return nil, fmt.Errorf("getting employee runtime endpoint: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(bridgeURLTTL)
	if err := o.db.Model(&sb).Updates(map[string]any{
		"external_id":           info.ExternalID,
		"bridge_url":            sandboxURL,
		"bridge_url_expires_at": expiresAt,
		"status":                "running",
		"last_active_at":        now,
	}).Error; err != nil {
		return nil, fmt.Errorf("updating sandbox: %w", err)
	}
	sb.ExternalID = info.ExternalID
	sb.BridgeURL = sandboxURL
	sb.BridgeURLExpiresAt = &expiresAt
	sb.Status = "running"
	sb.LastActiveAt = &now

	if err := o.waitForEmployeeRuntimeLive(ctx, &sb); err != nil {
		o.markSandboxError(ctx, &sb, map[string]any{
			"status":        "error",
			"error_message": "employee runtime not live",
		})
		return nil, fmt.Errorf("waiting for employee runtime: %w", err)
	}

	if err := o.cloneEmployeeSelectedRepositories(ctx, &sb, agent); err != nil {
		o.markSandboxError(ctx, &sb, map[string]any{
			"status":        "error",
			"error_message": fmt.Sprintf("repository cloning failed: %v", err),
		})
		return nil, fmt.Errorf("cloning employee repositories: %w", err)
	}

	disableProviderLifecycle(ctx, o.provider, &sb, info.ExternalID)
	logging.FromContext(ctx).InfoContext(ctx, "employee sandbox created",
		"sandbox_id", sb.ID, "external_id", info.ExternalID, "agent_id", agent.ID)
	return &sb, nil
}

func employeeSandboxEnvVars(cfg *config.Config, runtimeSecret string, sb *model.Sandbox, orgID uuid.UUID, agent *model.Agent, secrets *employeeruntime.StartupSecrets, gitIdentity *employeeGitIdentity) map[string]string {
	bridgeHost := "api.usehiveloop.com"
	proxyHost := "proxy.hiveloop.com"
	if cfg != nil {
		if cfg.BridgeHost != "" {
			bridgeHost = cfg.BridgeHost
		}
		if cfg.ProxyHost != "" {
			proxyHost = cfg.ProxyHost
		}
	}
	proxyBaseURL := "https://" + strings.TrimRight(proxyHost, "/") + "/v1"
	envVars := map[string]string{
		employeeruntime.EmployeeEnvRuntimeSecret:            runtimeSecret,
		employeeruntime.EmployeeEnvSlackBotToken:            secrets.SlackBotToken,
		employeeruntime.EmployeeEnvSlackAppToken:            secrets.SlackAppToken,
		employeeruntime.ProxyAPIKeyEnv:                      secrets.ProxyToken,
		employeeruntime.EmployeeEnvAgentModel:               employeeruntime.DefaultEmployeeModel,
		employeeruntime.EmployeeEnvAgentBaseURL:             proxyBaseURL,
		employeeruntime.EmployeeEnvAgentAPIKeyEnv:           employeeruntime.ProxyAPIKeyEnv,
		employeeruntime.EmployeeEnvAgentMultimodalModel:     employeeruntime.DefaultEmployeeMultimodalModel,
		employeeruntime.EmployeeEnvAgentMultimodalBaseURL:   proxyBaseURL,
		employeeruntime.EmployeeEnvAgentMultimodalAPIKeyEnv: employeeruntime.ProxyAPIKeyEnv,
		employeeruntime.EmployeeEnvEmployeeID:               agent.ID.String(),
		employeeruntime.EmployeeEnvCloudControlPlaneURL:     "https://" + bridgeHost,
		employeeruntime.EmployeeEnvBridgeAPIKey:             runtimeSecret,
		employeeruntime.EmployeeEnvUploadBearer:             runtimeSecret,
		employeeruntime.EmployeeEnvWorkspaceRoot:            "/workspace",
		employeeruntime.EmployeeEnvDBPath:                   "/app/data/employee-bridge.db",
		employeeruntime.EmployeeEnvRuntimeBindAddr:          fmt.Sprintf("0.0.0.0:%d", EmployeeSandboxPort),
		employeeruntime.EmployeeEnvSandboxID:                sb.ID.String(),
		employeeruntime.EmployeeEnvOrgID:                    orgID.String(),
		employeeruntime.EmployeeEnvAgentID:                  agent.ID.String(),
		employeeruntime.EmployeeEnvGitUsername:              employeeGitUsername(agent, gitIdentity),
		employeeruntime.EmployeeEnvGitEmail:                 employeeGitEmail(agent, gitIdentity),
		employeeruntime.EmployeeEnvGitCredentialsURL:        fmt.Sprintf("https://%s/internal/git-credentials/%s", bridgeHost, agent.ID),
		employeeruntime.EmployeeEnvGitHubNoKeyring:          "1",
		employeeruntime.EmployeeEnvBugsinkURL:               fmt.Sprintf("https://%s/internal/bugsink-proxy/%s", bridgeHost, agent.ID),
		employeeruntime.EmployeeEnvBugsinkToken:             runtimeSecret,
		employeeruntime.EmployeeEnvLinearURL:                fmt.Sprintf("https://%s/internal/linear-proxy/%s", bridgeHost, agent.ID),
		employeeruntime.EmployeeEnvLinearToken:              runtimeSecret,
	}
	setEmployeeDriveUploadURL(envVars, cfg, agent.ID, "employee")
	employeeSentryDSN := ""
	if cfg != nil {
		employeeSentryDSN = cfg.EmployeeSandboxSentryDSN
	}
	setSandboxSentryEnvVars(envVars, cfg, employeeSentryDSN)
	return envVars
}

func buildEmployeeSandboxName(agent *model.Agent) string {
	return fmt.Sprintf("hiveloop-employee-%s-%s-%d", sanitizeName(agent.Name), shortID(agent.ID), time.Now().Unix())
}

func (o *Orchestrator) waitForEmployeeRuntimeLive(ctx context.Context, sb *model.Sandbox) error {
	healthURL := strings.TrimRight(sb.BridgeURL, "/") + "/healthz"
	deadline := time.Now().Add(employeeHealthTimeout)
	client := &http.Client{Timeout: 5 * time.Second}
	attempt := 0

	logging.FromContext(ctx).InfoContext(ctx, "waiting for employee runtime", "sandbox_id", sb.ID)
	for time.Now().Before(deadline) {
		attempt++
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		resp, doErr := client.Do(req)
		if doErr != nil {
			logging.FromContext(ctx).DebugContext(ctx, "employee runtime probe transport error",
				"sandbox_id", sb.ID, "attempt", attempt, "error", doErr)
		} else {
			status := resp.StatusCode
			resp.Body.Close()
			if status == http.StatusOK {
				logging.FromContext(ctx).InfoContext(ctx, "employee runtime live",
					"sandbox_id", sb.ID, "attempts", attempt)
				return nil
			}
			logging.FromContext(ctx).DebugContext(ctx, "employee runtime probe non-200",
				"sandbox_id", sb.ID, "attempt", attempt, "status", status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(employeeHealthInterval):
		}
	}
	return fmt.Errorf("employee runtime not live within %s (%d attempts)", employeeHealthTimeout, attempt)
}

func (o *Orchestrator) RefreshEmployeeSandboxURL(ctx context.Context, sb *model.Sandbox) error {
	url, err := o.provider.GetEndpoint(ctx, sb.ExternalID, EmployeeSandboxPort)
	if err != nil {
		return fmt.Errorf("get employee sandbox endpoint: %w", err)
	}
	expiresAt := time.Now().Add(bridgeURLTTL)
	if err := o.db.Model(sb).Updates(map[string]any{
		"bridge_url":            url,
		"bridge_url_expires_at": expiresAt,
	}).Error; err != nil {
		return fmt.Errorf("update sandbox url: %w", err)
	}
	sb.BridgeURL = url
	sb.BridgeURLExpiresAt = &expiresAt
	return nil
}

func (o *Orchestrator) StartEmployeeSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.provider.StartSandbox(ctx, sb.ExternalID); err != nil {
		return fmt.Errorf("starting employee sandbox %s: %w", sb.ID, err)
	}
	if err := o.RefreshEmployeeSandboxURL(ctx, sb); err != nil {
		return err
	}
	now := time.Now()
	if err := o.db.Model(sb).Updates(map[string]any{
		"status":         string(StatusRunning),
		"last_active_at": now,
		"stopped_at":     nil,
		"error_message":  nil,
	}).Error; err != nil {
		return fmt.Errorf("mark employee sandbox running: %w", err)
	}
	sb.Status = string(StatusRunning)
	sb.LastActiveAt = &now
	sb.StoppedAt = nil
	sb.ErrorMessage = nil
	if err := o.waitForEmployeeRuntimeLive(ctx, sb); err != nil {
		return fmt.Errorf("waiting for employee runtime: %w", err)
	}
	return nil
}

func (o *Orchestrator) RestartEmployeeSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.StopSandbox(ctx, sb); err != nil {
		return err
	}
	return o.StartEmployeeSandbox(ctx, sb)
}

func (o *Orchestrator) NeedsURLRefresh(sb *model.Sandbox) bool {
	return o.needsURLRefresh(sb)
}
