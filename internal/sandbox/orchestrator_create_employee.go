package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const (
	EmployeeSandboxPort    = 7080
	employeeHealthTimeout  = 90 * time.Second
	employeeHealthInterval = 2 * time.Second
)

func (o *Orchestrator) CreateEmployeeSandbox(ctx context.Context, agent *model.Employee, secrets *employeeruntime.StartupSecrets) (*model.Sandbox, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("CreateEmployeeSandbox: agent must have org_id")
	}
	if secrets == nil || secrets.ProxyToken == "" {
		return nil, fmt.Errorf("CreateEmployeeSandbox: proxy token is required")
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

	snapshotID := o.cfg.SandboxesRuntimeBaseImage
	sb := model.Sandbox{
		OrgID:                  &orgID,
		EmployeeID:             &agent.ID,
		SnapshotID:             &snapshotID,
		ProviderID:             o.provider.ID(),
		EncryptedRuntimeSecret: encryptedSecret,
		Status:                 "creating",
	}
	if err := o.db.Create(&sb).Error; err != nil {
		return nil, fmt.Errorf("saving sandbox: %w", err)
	}

	bugsinkDashboardURL := employeeruntime.BugsinkDashboardBaseURL(ctx, o.db, orgID, *agent)
	envVars := employeeSandboxEnvVars(o.cfg, runtimeSecret, &sb, orgID, agent, secrets, gitIdentity, bugsinkDashboardURL)
	labels := map[string]string{
		"org_id":      orgID.String(),
		"sandbox_id":  sb.ID.String(),
		"employee_id": agent.ID.String(),
		"harness":     "employee-sandbox",
	}

	if _, usesWarmPool := o.provider.(WarmPoolCapable); usesWarmPool {
		if err := o.claimWarmRuntime(ctx, &sb, model.SandboxWarmSlotModeEmployee); err != nil {
			if delErr := o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error; delErr != nil {
				logging.FromContext(ctx).ErrorContext(ctx, "delete orphaned employee sandbox row after warm claim failure",
					"error", delErr, "sandbox_id", sb.ID)
			}
			return nil, err
		}
		if err := o.cloneEmployeeSelectedRepositories(ctx, &sb, agent); err != nil {
			o.markSandboxError(ctx, &sb, map[string]any{
				"status":        "error",
				"error_message": fmt.Sprintf("repository cloning failed: %v", err),
			})
			return nil, fmt.Errorf("cloning employee repositories: %w", err)
		}
		logging.FromContext(ctx).InfoContext(ctx, "employee sandbox claimed from warm pool",
			"sandbox_id", sb.ID, "external_id", sb.ExternalID, "employee_id", agent.ID)
		return &sb, nil
	}

	info, err := o.provider.CreateSandbox(ctx, CreateSandboxOpts{
		Name:        buildEmployeeSandboxName(agent),
		TemplateRef: snapshotID,
		EnvVars:     envVars,
		Labels:      labels,
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
	expiresAt := now.Add(runtimeURLTTL)
	if err := o.db.Model(&sb).Updates(map[string]any{
		"external_id":            info.ExternalID,
		"runtime_url":            sandboxURL,
		"runtime_url_expires_at": expiresAt,
		"status":                 "running",
		"last_active_at":         now,
	}).Error; err != nil {
		return nil, fmt.Errorf("updating sandbox: %w", err)
	}
	sb.ExternalID = info.ExternalID
	sb.RuntimeURL = sandboxURL
	sb.RuntimeURLExpiresAt = &expiresAt
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
		"sandbox_id", sb.ID, "external_id", info.ExternalID, "employee_id", agent.ID)
	return &sb, nil
}

func (o *Orchestrator) CreateSpecialistRuntimeSandbox(ctx context.Context, agent *model.Employee, secrets *employeeruntime.StartupSecrets) (*model.Sandbox, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("CreateSpecialistRuntimeSandbox: agent must have org_id")
	}
	if secrets == nil || secrets.ProxyToken == "" {
		return nil, fmt.Errorf("CreateSpecialistRuntimeSandbox: proxy token is required")
	}
	orgID := *agent.OrgID

	gitIdentity, err := o.loadAgentGitIdentity(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("loading specialist runtime git identity: %w", err)
	}
	runtimeSecret, err := generateRandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generating runtime secret: %w", err)
	}
	encryptedSecret, err := o.encKey.EncryptString(runtimeSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypting runtime secret: %w", err)
	}

	snapshotID := o.cfg.SandboxesRuntimeSpecialistImage
	sb := model.Sandbox{
		OrgID:                  &orgID,
		EmployeeID:             &agent.ID,
		SnapshotID:             &snapshotID,
		ProviderID:             o.provider.ID(),
		EncryptedRuntimeSecret: encryptedSecret,
		Status:                 "creating",
	}
	if err := o.db.Create(&sb).Error; err != nil {
		return nil, fmt.Errorf("saving specialist runtime sandbox: %w", err)
	}

	bugsinkDashboardURL := employeeruntime.BugsinkDashboardBaseURL(ctx, o.db, orgID, *agent)
	envVars := employeeSandboxEnvVars(o.cfg, runtimeSecret, &sb, orgID, agent, secrets, gitIdentity, bugsinkDashboardURL)
	envVars[employeeruntime.EmployeeEnvRuntimeMode] = "specialist"
	labels := map[string]string{
		"org_id":      orgID.String(),
		"sandbox_id":  sb.ID.String(),
		"employee_id": agent.ID.String(),
		"harness":     "runtime-specialist",
		"mode":        "specialist",
	}

	if _, usesWarmPool := o.provider.(WarmPoolCapable); usesWarmPool {
		if err := o.claimWarmRuntime(ctx, &sb, model.SandboxWarmSlotModeSpecialist); err != nil {
			if delErr := o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error; delErr != nil {
				logging.FromContext(ctx).ErrorContext(ctx, "delete orphaned specialist runtime sandbox row after warm claim failure",
					"error", delErr, "sandbox_id", sb.ID)
			}
			return nil, err
		}
		if err := o.cloneEmployeeSelectedRepositories(ctx, &sb, agent); err != nil {
			o.markSandboxError(ctx, &sb, map[string]any{
				"status":        "error",
				"error_message": fmt.Sprintf("repository cloning failed: %v", err),
			})
			return nil, fmt.Errorf("cloning specialist runtime repositories: %w", err)
		}
		logging.FromContext(ctx).InfoContext(ctx, "specialist runtime sandbox claimed from warm pool",
			"sandbox_id", sb.ID, "external_id", sb.ExternalID, "employee_id", agent.ID)
		return &sb, nil
	}

	info, err := o.provider.CreateSandbox(ctx, CreateSandboxOpts{
		Name:        buildSpecialistRuntimeSandboxName(agent),
		TemplateRef: snapshotID,
		EnvVars:     envVars,
		Labels:      labels,
	})
	if err != nil {
		if delErr := o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error; delErr != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "delete orphaned specialist runtime sandbox row after provider create failure",
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
		return nil, fmt.Errorf("getting specialist runtime endpoint: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(runtimeURLTTL)
	if err := o.db.Model(&sb).Updates(map[string]any{
		"external_id":            info.ExternalID,
		"runtime_url":            sandboxURL,
		"runtime_url_expires_at": expiresAt,
		"status":                 "running",
		"last_active_at":         now,
	}).Error; err != nil {
		return nil, fmt.Errorf("updating specialist runtime sandbox: %w", err)
	}
	sb.ExternalID = info.ExternalID
	sb.RuntimeURL = sandboxURL
	sb.RuntimeURLExpiresAt = &expiresAt
	sb.Status = "running"
	sb.LastActiveAt = &now

	if err := o.waitForEmployeeRuntimeLive(ctx, &sb); err != nil {
		o.markSandboxError(ctx, &sb, map[string]any{
			"status":        "error",
			"error_message": "specialist runtime not live",
		})
		return nil, fmt.Errorf("waiting for specialist runtime: %w", err)
	}
	if err := o.cloneEmployeeSelectedRepositories(ctx, &sb, agent); err != nil {
		o.markSandboxError(ctx, &sb, map[string]any{
			"status":        "error",
			"error_message": fmt.Sprintf("repository cloning failed: %v", err),
		})
		return nil, fmt.Errorf("cloning specialist runtime repositories: %w", err)
	}
	disableProviderLifecycle(ctx, o.provider, &sb, info.ExternalID)
	logging.FromContext(ctx).InfoContext(ctx, "specialist runtime sandbox created",
		"sandbox_id", sb.ID, "external_id", info.ExternalID, "employee_id", agent.ID)
	return &sb, nil
}
