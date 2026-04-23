package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
)

func (o *Orchestrator) createPoolSandbox(ctx context.Context) (*model.Sandbox, error) {
	bridgeAPIKey, err := generateRandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generating bridge api key: %w", err)
	}
	encryptedKey, err := o.encKey.EncryptString(bridgeAPIKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting bridge api key: %w", err)
	}

	sb := model.Sandbox{
		SandboxType:           "shared",
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "creating",
	}
	if err := o.db.Create(&sb).Error; err != nil {
		return nil, fmt.Errorf("saving pool sandbox record: %w", err)
	}

	webhookURL := fmt.Sprintf("https://%s/internal/webhooks/bridge/%s", o.cfg.BridgeHost, sb.ID)
	envVars := baseEnvVars(o.cfg, bridgeAPIKey, sb.ID, webhookURL)

	snapshotID := o.cfg.BridgeBaseImagePrefix
	name := fmt.Sprintf("hiveloop-pool-%s", shortID(sb.ID))

	labels := map[string]string{
		"sandbox_type": "pool",
		"sandbox_id":   sb.ID.String(),
	}

	info, err := o.provider.CreateSandbox(ctx, CreateSandboxOpts{
		Name:       name,
		SnapshotID: snapshotID,
		EnvVars:    envVars,
		Labels:     labels,
	})
	if err != nil {
		o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		return nil, fmt.Errorf("creating pool sandbox via provider: %w", err)
	}

	disableProviderLifecycle(ctx, o.provider, &sb, info.ExternalID)

	bridgeURL, err := o.provider.GetEndpoint(ctx, info.ExternalID, BridgePort)
	if err != nil {
		o.db.Model(&sb).Updates(map[string]any{
			"external_id":   info.ExternalID,
			"status":        "error",
			"error_message": fmt.Sprintf("failed to get endpoint: %v", err),
		})
		return nil, fmt.Errorf("getting pool sandbox endpoint: %w", err)
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
		return nil, fmt.Errorf("updating pool sandbox record: %w", err)
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
		return nil, fmt.Errorf("waiting for pool bridge: %w", err)
	}

	slog.Info("pool sandbox created",
		"sandbox_id", sb.ID,
		"external_id", info.ExternalID,
	)

	return &sb, nil
}

func (o *Orchestrator) createSystemSandbox(ctx context.Context) (*model.Sandbox, error) {
	bridgeAPIKey, err := generateRandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generating bridge api key: %w", err)
	}
	encryptedKey, err := o.encKey.EncryptString(bridgeAPIKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting bridge api key: %w", err)
	}

	sb := model.Sandbox{
		SandboxType:           "system",
		EncryptedBridgeAPIKey: encryptedKey,
		Status:                "creating",
	}
	if err := o.db.Create(&sb).Error; err != nil {
		return nil, fmt.Errorf("saving system sandbox record: %w", err)
	}

	webhookURL := fmt.Sprintf("https://%s/internal/webhooks/bridge/%s", o.cfg.BridgeHost, sb.ID)
	envVars := baseEnvVars(o.cfg, bridgeAPIKey, sb.ID, webhookURL)

	snapshotID := o.cfg.BridgeBaseImagePrefix
	name := fmt.Sprintf("hiveloop-system-%s", shortID(sb.ID))

	labels := map[string]string{
		"sandbox_type": "system",
		"sandbox_id":   sb.ID.String(),
	}

	info, err := o.provider.CreateSandbox(ctx, CreateSandboxOpts{
		Name:       name,
		SnapshotID: snapshotID,
		EnvVars:    envVars,
		Labels:     labels,
	})
	if err != nil {
		o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		return nil, fmt.Errorf("creating system sandbox via provider: %w", err)
	}

	disableProviderLifecycle(ctx, o.provider, &sb, info.ExternalID)

	bridgeURL, err := o.provider.GetEndpoint(ctx, info.ExternalID, BridgePort)
	if err != nil {
		o.db.Model(&sb).Updates(map[string]any{
			"external_id":   info.ExternalID,
			"status":        "error",
			"error_message": fmt.Sprintf("failed to get endpoint: %v", err),
		})
		return nil, fmt.Errorf("getting system sandbox endpoint: %w", err)
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
		return nil, fmt.Errorf("updating system sandbox record: %w", err)
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
		return nil, fmt.Errorf("waiting for system bridge: %w", err)
	}

	slog.Info("system sandbox created",
		"sandbox_id", sb.ID,
		"external_id", info.ExternalID,
	)

	return &sb, nil
}
