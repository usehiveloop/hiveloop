package sandbox

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
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

	runtimeSecret, err := generateRandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generating runtime secret: %w", err)
	}
	encryptedSecret, err := o.encKey.EncryptString(runtimeSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypting runtime secret: %w", err)
	}

	sb := model.Sandbox{
		OrgID:                 &orgID,
		AgentID:               &agent.ID,
		EncryptedBridgeAPIKey: encryptedSecret,
		Status:                "creating",
	}
	if err := o.db.Create(&sb).Error; err != nil {
		return nil, fmt.Errorf("saving sandbox: %w", err)
	}

	envVars := employeeSandboxEnvVars(o.cfg, runtimeSecret, &sb, orgID, agent, secrets)
	labels := map[string]string{
		"org_id":     orgID.String(),
		"sandbox_id": sb.ID.String(),
		"agent_id":   agent.ID.String(),
		"harness":    "employee-sandbox",
	}

	info, err := o.provider.CreateSandbox(ctx, CreateSandboxOpts{
		Name:       buildEmployeeSandboxName(agent),
		SnapshotID: o.cfg.EmployeeSandboxBaseImagePrefix,
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

	disableProviderLifecycle(ctx, o.provider, &sb, info.ExternalID)
	logging.FromContext(ctx).InfoContext(ctx, "employee sandbox created",
		"sandbox_id", sb.ID, "external_id", info.ExternalID, "agent_id", agent.ID)
	return &sb, nil
}

func employeeSandboxEnvVars(cfg *config.Config, runtimeSecret string, sb *model.Sandbox, orgID uuid.UUID, agent *model.Agent, secrets *employeeruntime.StartupSecrets) map[string]string {
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
		"RUNTIME_SECRET":               runtimeSecret,
		"SLACK_BOT_TOKEN":              secrets.SlackBotToken,
		"SLACK_APP_TOKEN":              secrets.SlackAppToken,
		employeeruntime.ProxyAPIKeyEnv: secrets.ProxyToken,
		"AGENT_MODEL":                  employeeruntime.DefaultEmployeeModel,
		"AGENT_BASE_URL":               proxyBaseURL,
		"AGENT_API_KEY_ENV":            employeeruntime.ProxyAPIKeyEnv,
		"AGENT_MULTIMODAL_MODEL":       employeeruntime.DefaultEmployeeMultimodalModel,
		"AGENT_MULTIMODAL_BASE_URL":    proxyBaseURL,
		"AGENT_MULTIMODAL_API_KEY_ENV": employeeruntime.ProxyAPIKeyEnv,
		"EMPLOYEE_ID":                  agent.ID.String(),
		"CLOUD_CONTROL_PLANE_URL":      "https://" + bridgeHost,
		"BRIDGE_API_KEY":               runtimeSecret,
		"WORKSPACE_ROOT":               "/workspace",
		"DB_PATH":                      "/app/data/employee-bridge.db",
		"RUNTIME_BIND_ADDR":            fmt.Sprintf("0.0.0.0:%d", EmployeeSandboxPort),
		"HIVELOOP_SANDBOX_ID":          sb.ID.String(),
		"HIVELOOP_ORG_ID":              orgID.String(),
		"HIVELOOP_AGENT_ID":            agent.ID.String(),
		"HIVELOOP_GIT_USERNAME":        sanitizeName(agent.Name),
		"HIVELOOP_GIT_EMAIL":           sanitizeName(agent.Name) + "@usehiveloop.com",
		"HIVELOOP_GIT_CREDENTIALS_URL": fmt.Sprintf("https://%s/internal/git-credentials/%s", bridgeHost, agent.ID),
		"GH_NO_KEYRING":                "1",
	}
	if cfg != nil && strings.TrimSpace(cfg.SentryDSN) != "" {
		envVars["SENTRY_DSN"] = cfg.SentryDSN
		envVars["SENTRY_ENVIRONMENT"] = cfg.Environment
		envVars["SENTRY_SAMPLE_RATE"] = "1"
		envVars["SENTRY_TRACES_SAMPLE_RATE"] = strconv.FormatFloat(cfg.SentryTracesSampleRate, 'f', -1, 64)
		envVars["SENTRY_ENABLE_LOGS"] = "true"
		if strings.TrimSpace(cfg.SentryRelease) != "" {
			envVars["SENTRY_RELEASE"] = cfg.SentryRelease
		}
	}
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

func (o *Orchestrator) NeedsURLRefresh(sb *model.Sandbox) bool {
	return o.needsURLRefresh(sb)
}
