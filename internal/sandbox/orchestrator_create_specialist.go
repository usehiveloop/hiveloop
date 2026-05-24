package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) createSandbox(ctx context.Context, org *model.Org, agent *model.Employee, extraEnv map[string]string) (*model.Sandbox, error) {
	gitIdentity, err := o.loadAgentGitIdentity(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("loading sandbox git identity: %w", err)
	}

	bridgeAPIKey, err := generateRandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generating bridge api key: %w", err)
	}
	encryptedKey, err := o.encKey.EncryptString(bridgeAPIKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting bridge api key: %w", err)
	}

	sb := model.Sandbox{
		OrgID:                 &org.ID,
		ProviderID:            o.provider.ID(),
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "creating",
	}
	if agent != nil {
		sb.EmployeeID = &agent.ID
		if agent.SandboxTemplateID != nil {
			sb.SandboxTemplateID = agent.SandboxTemplateID
		}
	}
	if err := o.db.Create(&sb).Error; err != nil {
		return nil, fmt.Errorf("saving sandbox record: %w", err)
	}

	owningEmployee, err := o.loadOwningEmployee(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("loading owning employee: %w", err)
	}

	webhookURL := fmt.Sprintf("https://%s/internal/webhooks/employee/%s", o.cfg.SpecialistSandboxHost, sb.ID)
	envVars := baseEnvVars(o.cfg, bridgeAPIKey, sb.ID, webhookURL)
	setOrgEnvVars(envVars, org.ID)
	setAgentEnvVars(envVars, agent, o.cfg)
	setDriveEndpoint(envVars, sb.ID, o.cfg)
	setAssetsUploadURL(envVars, o.cfg)

	if owningEmployee != nil {
		o.mergeUserEnvVars(ctx, envVars, owningEmployee.EncryptedEnvVars)
	}
	if agent != nil {
		o.mergeUserEnvVars(ctx, envVars, agent.EncryptedEnvVars)
	}
	for key, value := range extraEnv {
		envVars[key] = value
	}
	setGitIdentityEnvVars(envVars, agent, gitIdentity)
	setUploadBearer(envVars, bridgeAPIKey)

	templateRef := o.resolveTemplateRef(agent)
	cpu, memory, disk := o.resolveTemplateResources(agent)
	name := o.buildSandboxName(agent)

	labels := map[string]string{
		"org_id":     org.ID.String(),
		"sandbox_id": sb.ID.String(),
	}
	if agent != nil {
		labels["employee_id"] = agent.ID.String()
	}

	info, err := o.provider.CreateSandbox(ctx, CreateSandboxOpts{
		Name:        name,
		TemplateRef: templateRef,
		EnvVars:     envVars,
		Labels:      labels,
		CPU:         cpu,
		Memory:      memory,
		Disk:        disk,
	})
	if err != nil {
		o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		return nil, fmt.Errorf("creating sandbox via provider: %w", err)
	}

	bridgeURL, err := o.provider.GetEndpoint(ctx, info.ExternalID, BridgePort)
	if err != nil {
		o.db.Model(&sb).Updates(map[string]any{
			"external_id":   info.ExternalID,
			"status":        "error",
			"error_message": fmt.Sprintf("failed to get endpoint: %v", err),
		})
		return nil, fmt.Errorf("getting sandbox endpoint: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(bridgeURLTTL)
	if err := o.db.Model(&sb).Updates(map[string]any{
		"external_id":           info.ExternalID,
		"bridge_url":            bridgeURL,
		"bridge_url_expires_at": expiresAt,
		"status":                "running",
		"last_active_at":        now,
	}).Error; err != nil {
		return nil, fmt.Errorf("updating sandbox record: %w", err)
	}

	sb.ExternalID = info.ExternalID
	sb.BridgeURL = bridgeURL
	sb.BridgeURLExpiresAt = &expiresAt
	sb.Status = "running"
	sb.LastActiveAt = &now

	// Older templates may not have the harness config dir; create it so the
	// runtime does not crash on first read.
	if _, execErr := o.provider.ExecuteCommand(ctx, info.ExternalID, "mkdir -p /work/.opencode"); execErr != nil {
		logging.Capture(ctx, fmt.Errorf("create bridge config dirs sandbox %s: %w", sb.ID, execErr))
	}

	if err := o.waitForBridgeHealthy(ctx, &sb); err != nil {
		o.db.Model(&sb).Updates(map[string]any{
			"status":        "error",
			"error_message": fmt.Sprintf("bridge failed to start: %v", err),
		})
		return nil, fmt.Errorf("waiting for bridge: %w", err)
	}

	disableProviderLifecycle(ctx, o.provider, &sb, info.ExternalID)

	if agent != nil && len(agent.SetupCommands) > 0 {
		if err := o.runSetupCommands(ctx, &sb, agent.SetupCommands); err != nil {
			o.db.Model(&sb).Updates(map[string]any{
				"status":        "error",
				"error_message": fmt.Sprintf("setup commands failed: %v", err),
			})
			return nil, fmt.Errorf("setup commands failed: %w", err)
		}
	}

	if agent != nil {
		cloneSource := cloneAgentWithInheritedResources(agent, owningEmployee)
		if err := o.cloneAgentRepositories(ctx, &sb, cloneSource); err != nil {
			o.db.Model(&sb).Updates(map[string]any{
				"status":        "error",
				"error_message": fmt.Sprintf("repository cloning failed: %v", err),
			})
			return nil, fmt.Errorf("cloning repositories: %w", err)
		}
		if owningEmployee != nil {
			if err := o.cloneEmployeeSelectedRepositories(ctx, &sb, owningEmployee); err != nil {
				o.db.Model(&sb).Updates(map[string]any{
					"status":        "error",
					"error_message": fmt.Sprintf("employee repository cloning failed: %v", err),
				})
				return nil, fmt.Errorf("cloning employee selected repositories: %w", err)
			}
		}
	}

	logging.FromContext(ctx).InfoContext(ctx, "sandbox created",
		"sandbox_id", sb.ID,
		"external_id", info.ExternalID,
	)

	return &sb, nil
}

func (o *Orchestrator) resolveTemplateRef(agent *model.Employee) string {
	if agent != nil && agent.SandboxTemplateID != nil {
		var tmpl model.SandboxTemplate
		if err := o.db.Where("id = ?", *agent.SandboxTemplateID).First(&tmpl).Error; err == nil {
			if tmpl.ExternalID != nil && tmpl.BuildStatus == "ready" {
				return *tmpl.ExternalID
			}
		}
	}
	return o.cfg.SandboxesRuntimeSpecialistImagePrefix
}

func (o *Orchestrator) resolveTemplateResources(agent *model.Employee) (int, int, int) {
	if agent == nil || agent.SandboxTemplateID == nil {
		return 0, 0, 0
	}
	var tmpl model.SandboxTemplate
	if err := o.db.Where("id = ?", *agent.SandboxTemplateID).First(&tmpl).Error; err != nil {
		return 0, 0, 0
	}
	if tmpl.ExternalID == nil || tmpl.BuildStatus != "ready" {
		return 0, 0, 0
	}
	if sz, ok := model.TemplateSizes[tmpl.Size]; ok {
		return sz.CPU, sz.Memory, sz.Disk
	}
	return 0, 0, 0
}

func (o *Orchestrator) buildSandboxName(agent *model.Employee) string {
	ts := time.Now().Unix()
	if agent != nil {
		safeName := sanitizeName(agent.Name)
		return fmt.Sprintf("hivy-specialist-%s-%s-%d", safeName, shortID(agent.ID), ts)
	}
	return fmt.Sprintf("hivy-specialist-%d", ts)
}
