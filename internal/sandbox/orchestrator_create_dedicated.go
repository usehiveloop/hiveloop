package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/turso"
)

func (o *Orchestrator) createSandbox(ctx context.Context, org *model.Org, agent *model.Agent) (*model.Sandbox, error) {
	var storageURL, authToken string
	if o.turso != nil {
		var err error
		storageURL, authToken, err = o.turso.EnsureStorage(ctx, org.ID)
		if err != nil {
			slog.Warn("turso storage provisioning failed, continuing without libsql", "error", err)
		}
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
		SandboxType:           "dedicated",
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "creating",
	}
	if agent != nil {
		sb.AgentID = &agent.ID
		if agent.SandboxTemplateID != nil {
			sb.SandboxTemplateID = agent.SandboxTemplateID
		}
	}
	if err := o.db.Create(&sb).Error; err != nil {
		return nil, fmt.Errorf("saving sandbox record: %w", err)
	}

	webhookURL := fmt.Sprintf("https://%s/internal/webhooks/bridge/%s", o.cfg.BridgeHost, sb.ID)
	envVars := baseEnvVars(o.cfg, bridgeAPIKey, sb.ID, webhookURL)
	setOrgEnvVars(envVars, org.ID)
	setAgentEnvVars(envVars, agent, o.cfg)
	setDriveEndpoint(envVars, sb.ID, o.cfg)
	if storageURL != "" {
		envVars["BRIDGE_STORAGE_URL"] = storageURL
		envVars["BRIDGE_STORAGE_AUTH_TOKEN"] = authToken
	}

	if agent != nil {
		o.mergeUserEnvVars(envVars, agent.EncryptedEnvVars)
	}

	snapshotID := o.resolveSnapshot(agent)
	name := o.buildSandboxName(agent)

	labels := map[string]string{
		"org_id":       org.ID.String(),
		"sandbox_type": "dedicated",
		"sandbox_id":   sb.ID.String(),
	}
	if agent != nil {
		labels["agent_id"] = agent.ID.String()
	}

	info, err := o.provider.CreateSandbox(ctx, CreateSandboxOpts{
		Name:       name,
		SnapshotID: snapshotID,
		EnvVars:    envVars,
		Labels:     labels,
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

	slog.Info("got bridge endpoint",
		"sandbox_id", sb.ID,
		"external_id", info.ExternalID,
		"bridge_url", bridgeURL,
	)

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

	if _, execErr := o.provider.ExecuteCommand(ctx, info.ExternalID, "mkdir -p /home/daytona/.bridge"); execErr != nil {
		slog.Warn("failed to create bridge storage dir", "sandbox_id", sb.ID, "error", execErr)
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
		if err := o.cloneAgentRepositories(ctx, &sb, agent); err != nil {
			o.db.Model(&sb).Updates(map[string]any{
				"status":        "error",
				"error_message": fmt.Sprintf("repository cloning failed: %v", err),
			})
			return nil, fmt.Errorf("cloning repositories: %w", err)
		}
	}

	slog.Info("sandbox created",
		"sandbox_id", sb.ID,
		"external_id", info.ExternalID,
		"type", "dedicated",
	)

	return &sb, nil
}

func (o *Orchestrator) resolveSnapshot(agent *model.Agent) string {
	if agent != nil && agent.SandboxTemplateID != nil {
		var tmpl model.SandboxTemplate
		if err := o.db.Where("id = ?", *agent.SandboxTemplateID).First(&tmpl).Error; err == nil {
			if tmpl.ExternalID != nil && tmpl.BuildStatus == "ready" {
				return *tmpl.ExternalID
			}
		}
	}
	return o.cfg.BridgeBaseDedicatedImagePrefix
}

func (o *Orchestrator) buildSandboxName(agent *model.Agent) string {
	ts := time.Now().Unix()
	if agent != nil {
		safeName := sanitizeName(agent.Name)
		return fmt.Sprintf("hiveloop-ded-%s-%s-%d", safeName, shortID(agent.ID), ts)
	}
	return fmt.Sprintf("hiveloop-ded-%d", ts)
}
