package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ziraloop/ziraloop/internal/bridge"
	"github.com/ziraloop/ziraloop/internal/config"
	"github.com/ziraloop/ziraloop/internal/crypto"
	"github.com/ziraloop/ziraloop/internal/model"
	"github.com/ziraloop/ziraloop/internal/turso"
)

const (
	// BridgePort is the fixed port Bridge listens on inside every sandbox.
	BridgePort = 25434

	// bridgeHealthTimeout is the max time to wait for Bridge to become healthy.
	bridgeHealthTimeout = 90 * time.Second

	// bridgeHealthInterval is the polling interval for Bridge health checks.
	bridgeHealthInterval = 2 * time.Second

	// bridgeURLRefreshBuffer is how early we refresh the pre-auth URL before it expires.
	bridgeURLRefreshBuffer = 5 * time.Minute

	// bridgeURLTTL is how long we assume a pre-auth URL is valid.
	// Daytona signed URLs last ~60 minutes; we store 55 to refresh early.
	bridgeURLTTL = 55 * time.Minute

	// healthCheckInterval is how often the background health checker runs.
	healthCheckInterval = 30 * time.Second
)

// baseEnvVars returns the environment variables common to all sandbox types.
// bridgeAPIKey is the per-sandbox control plane key.
// sandboxID is always available and included in every sandbox.
// webhookURL may be empty for sandbox types that don't use webhooks.
func baseEnvVars(cfg *config.Config, bridgeAPIKey string, sandboxID uuid.UUID, webhookURL string) map[string]string {
	envVars := map[string]string{
		"BRIDGE_CONTROL_PLANE_API_KEY": bridgeAPIKey,
		"BRIDGE_LISTEN_ADDR":           fmt.Sprintf("0.0.0.0:%d", BridgePort),
		"BRIDGE_LOG_FORMAT":            "json",
		"BRIDGE_STORAGE_PATH":          "/home/daytona/.bridge/storage",
		"BRIDGE_WEB_URL":               fmt.Sprintf("https://%s/spider", cfg.BridgeHost),
		"ZIRALOOP_SANDBOX_ID":          sandboxID.String(),
	}
	if webhookURL != "" {
		envVars["BRIDGE_WEBHOOK_URL"] = webhookURL
	}
	return envVars
}

// setOrgEnvVars adds org-level environment variables to the env map.
func setOrgEnvVars(envVars map[string]string, orgID uuid.UUID) {
	envVars["ZIRALOOP_ORG_ID"] = orgID.String()
}

// setAgentEnvVars adds agent-level environment variables to the env map.
func setAgentEnvVars(envVars map[string]string, agent *model.Agent, cfg *config.Config) {
	if agent == nil {
		return
	}
	envVars["ZIRALOOP_AGENT_ID"] = agent.ID.String()
	envVars["ZIRALOOP_GIT_CREDENTIALS_URL"] = fmt.Sprintf("https://%s/internal/git-credentials/%s", cfg.BridgeHost, agent.ID)
	envVars["ZIRALOOP_RAILWAY_API_URL"] = fmt.Sprintf("https://%s/internal/railway-proxy/%s", cfg.BridgeHost, agent.ID)
	envVars["ZIRALOOP_RAILWAY_API_KEY"] = envVars["BRIDGE_CONTROL_PLANE_API_KEY"]
	envVars["ZIRALOOP_VERCEL_API_KEY"] = envVars["BRIDGE_CONTROL_PLANE_API_KEY"]
	envVars["GH_NO_KEYRING"] = "1"
}

// setDriveEndpoint sets the ZIRALOOP_DRIVE_ENDPOINT env var once the sandbox ID is known.
func setDriveEndpoint(envVars map[string]string, sandboxID uuid.UUID, cfg *config.Config) {
	envVars["ZIRALOOP_DRIVE_ENDPOINT"] = fmt.Sprintf("https://%s/internal/sandbox-drive/%s", cfg.BridgeHost, sandboxID)
}

// Orchestrator manages sandbox lifecycle — creating, starting, stopping sandboxes
// and providing BridgeClients to talk to them.
type Orchestrator struct {
	db       *gorm.DB
	provider Provider
	turso    *turso.Provisioner
	encKey   *crypto.SymmetricKey
	cfg      *config.Config
}

// NewOrchestrator creates a sandbox orchestrator.
func NewOrchestrator(db *gorm.DB, provider Provider, turso *turso.Provisioner, encKey *crypto.SymmetricKey, cfg *config.Config) *Orchestrator {
	return &Orchestrator{
		db:       db,
		provider: provider,
		turso:    turso,
		encKey:   encKey,
		cfg:      cfg,
	}
}

// AssignPoolSandbox assigns a pool sandbox to a shared agent.
// If the agent already has a sandbox assigned, it returns that one (waking if needed).
// Otherwise, it picks the least-loaded pool sandbox under the resource threshold,
// or creates a new one on demand.
func (o *Orchestrator) AssignPoolSandbox(ctx context.Context, agent *model.Agent) (*model.Sandbox, error) {
	// If agent already has a sandbox assigned, try to use it
	if agent.SandboxID != nil {
		var existing model.Sandbox
		if err := o.db.Where("id = ?", *agent.SandboxID).First(&existing).Error; err == nil {
			// Verify it still exists in the provider
			if err := o.verifySandboxExists(ctx, &existing); err == nil {
				switch existing.Status {
				case "running":
					return &existing, nil
				default:
					// stopped, creating, starting, error — try to wake
					woken, err := o.WakeSandbox(ctx, &existing)
					if err == nil {
						return woken, nil
					}
					slog.Warn("failed to wake assigned sandbox, will reassign",
						"sandbox_id", existing.ID, "error", err)
				}
			} else {
				slog.Warn("assigned sandbox stale, will reassign",
					"sandbox_id", existing.ID, "error", err)
			}
		}
		// Clear stale assignment
		o.db.Model(agent).Update("sandbox_id", nil)
		agent.SandboxID = nil
	}

	// Select from pool: lowest memory usage under threshold, with row-level lock
	threshold := o.cfg.PoolSandboxResourceThreshold
	var sb model.Sandbox
	err := o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(`
			SELECT * FROM sandboxes
			WHERE sandbox_type = 'shared'
			  AND status = 'running'
			  AND (memory_limit_bytes = 0 OR (memory_used_bytes * 100.0 / memory_limit_bytes) < ?)
			ORDER BY CASE WHEN memory_limit_bytes = 0 THEN 0 ELSE (memory_used_bytes * 100.0 / memory_limit_bytes) END ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		`, threshold).Scan(&sb).Error; err != nil {
			return err
		}

		if sb.ID == uuid.Nil {
			return gorm.ErrRecordNotFound
		}

		// Assign agent to this sandbox
		if err := tx.Model(agent).Update("sandbox_id", sb.ID).Error; err != nil {
			return err
		}
		agent.SandboxID = &sb.ID
		return nil
	})

	if err == nil {
		return &sb, nil
	}

	if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("selecting shared sandbox: %w", err)
	}

	// No available sandbox — create one on demand
	newSb, err := o.createPoolSandbox(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating shared sandbox on demand: %w", err)
	}

	// Assign agent to the new sandbox
	o.db.Model(agent).Update("sandbox_id", newSb.ID)
	agent.SandboxID = &newSb.ID

	return newSb, nil
}

// verifySandboxExists checks if the sandbox's external ID still exists in the provider.
func (o *Orchestrator) verifySandboxExists(ctx context.Context, sb *model.Sandbox) error {
	if sb.ExternalID == "" {
		return fmt.Errorf("no external ID")
	}
	_, err := o.provider.GetEndpoint(ctx, sb.ExternalID, BridgePort)
	return err
}

// createPoolSandbox provisions a new sandbox for the global pool.
// No org, no identity — pool sandboxes are cross-tenant.
func (o *Orchestrator) createPoolSandbox(ctx context.Context) (*model.Sandbox, error) {
	// Generate and encrypt Bridge API key
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
	name := fmt.Sprintf("zira-pool-%s", shortID(sb.ID))

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

	// Disable provider-managed auto-stop and auto-archive. Lifecycle for pool
	// sandboxes is driven by the periodic SandboxLifecycle task (10m idle →
	// stop, 24h stopped → archive). intervalMinutes=0 disables each policy.
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

	// Ensure Bridge storage directory exists (snapshot may not have it)
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

// EnsureSystemSandbox returns the singleton system sandbox, provisioning or
// waking it if needed. After ensuring the sandbox is running, it bulk-binds
// every is_system=true agent row to that sandbox by setting their sandbox_id.
//
// Idempotent — safe to call on every server startup and from the periodic
// SystemAgentSync task. Pushing the agent definitions to Bridge is the
// caller's responsibility (see Pusher.PushAllSystemAgents).
func (o *Orchestrator) EnsureSystemSandbox(ctx context.Context) (*model.Sandbox, error) {
	var sb model.Sandbox
	err := o.db.Where("sandbox_type = ?", "system").First(&sb).Error

	switch {
	case err == gorm.ErrRecordNotFound:
		newSb, createErr := o.createSystemSandbox(ctx)
		if createErr != nil {
			return nil, fmt.Errorf("creating system sandbox: %w", createErr)
		}
		sb = *newSb

	case err != nil:
		return nil, fmt.Errorf("looking up system sandbox: %w", err)

	default:
		// Existing row — verify it's still alive at the provider.
		if vErr := o.verifySandboxExists(ctx, &sb); vErr != nil {
			slog.Warn("system sandbox stale at provider, recreating",
				"sandbox_id", sb.ID, "error", vErr)
			o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
			newSb, createErr := o.createSystemSandbox(ctx)
			if createErr != nil {
				return nil, fmt.Errorf("recreating system sandbox: %w", createErr)
			}
			sb = *newSb
		} else if sb.Status != "running" {
			woken, wakeErr := o.WakeSandbox(ctx, &sb)
			if wakeErr != nil {
				return nil, fmt.Errorf("waking system sandbox: %w", wakeErr)
			}
			sb = *woken
		}
	}

	// Bind every system agent row to this sandbox so the existing
	// `if agent.SandboxID == nil` branches in handlers naturally no-op.
	if err := o.db.Model(&model.Agent{}).
		Where("is_system = true").
		Update("sandbox_id", sb.ID).Error; err != nil {
		return nil, fmt.Errorf("binding system agents to sandbox: %w", err)
	}

	return &sb, nil
}

// createSystemSandbox provisions a fresh sandbox with sandbox_type='system'.
// Mirrors createPoolSandbox but with the system tag and a stable name.
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
	name := fmt.Sprintf("zira-system-%s", shortID(sb.ID))

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

	// Disable Daytona's auto-stop AND auto-archive on this sandbox. The system
	// sandbox must stay running indefinitely and must never be archived by the
	// provider. Convention: intervalMinutes=0 disables each policy.
	// Non-fatal — lifecycle is reconciled by periodic tasks.
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

// ReleasePoolSandbox clears an agent's sandbox assignment.
func (o *Orchestrator) ReleasePoolSandbox(ctx context.Context, agent *model.Agent) error {
	if agent.SandboxID == nil {
		return nil
	}

	o.db.Model(agent).Update("sandbox_id", nil)
	agent.SandboxID = nil

	return nil
}

// CreateDedicatedSandbox spins up a new sandbox for a dedicated agent.
// Synchronous — blocks until running or returns an error.
func (o *Orchestrator) CreateDedicatedSandbox(ctx context.Context, agent *model.Agent) (*model.Sandbox, error) {
	if agent.OrgID == nil {
		return nil, fmt.Errorf("cannot create dedicated sandbox for agent without org_id")
	}
	var org model.Org
	if err := o.db.Where("id = ?", *agent.OrgID).First(&org).Error; err != nil {
		return nil, fmt.Errorf("loading org: %w", err)
	}

	return o.createSandbox(ctx, &org, agent)
}


// GetBridgeClient returns a BridgeClient connected to the sandbox.
// This is the single chokepoint for all Bridge interactions — it guarantees
// the sandbox is active (waking stopped sandboxes, unarchiving archived
// sandboxes) before returning a client, and refreshes the pre-auth URL if
// it's about to expire.
func (o *Orchestrator) GetBridgeClient(ctx context.Context, sb *model.Sandbox) (*bridge.BridgeClient, error) {
	// Decrypt the Bridge API key
	apiKey, err := o.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting bridge api key: %w", err)
	}

	// Ensure the sandbox is running before anyone tries to talk to Bridge.
	// Handles stopped → wake, archived → unarchive transparently so callers
	// don't need to know about the lifecycle state machine.
	if _, err := o.EnsureSandboxActive(ctx, sb); err != nil {
		return nil, fmt.Errorf("ensuring sandbox active: %w", err)
	}

	// Check if URL needs refresh (may have been cleared by stop/archive).
	if o.needsURLRefresh(sb) {
		if err := o.refreshBridgeURL(ctx, sb); err != nil {
			return nil, fmt.Errorf("refreshing bridge URL: %w", err)
		}
	}

	// Bump last_active_at — any bridge interaction counts as activity, which
	// resets the 10m idle timer for the sandbox lifecycle task.
	o.touchLastActive(sb)

	return bridge.NewBridgeClient(sb.BridgeURL, apiKey), nil
}

// touchLastActive bumps the sandbox's LastActiveAt column asynchronously so
// hot callers (GetBridgeClient is invoked per-request) don't pay the DB
// round-trip synchronously. The 10m idle window is coarse enough that a
// second of drift from the fire-and-forget update is harmless.
func (o *Orchestrator) touchLastActive(sb *model.Sandbox) {
	now := time.Now()
	sb.LastActiveAt = &now
	go func(id uuid.UUID) {
		if err := o.db.Model(&model.Sandbox{}).
			Where("id = ?", id).
			Update("last_active_at", now).Error; err != nil {
			slog.Debug("touchLastActive update failed", "sandbox_id", id, "error", err)
		}
	}(sb.ID)
}

// StopSandbox stops a running sandbox. Records the StoppedAt timestamp so the
// periodic SandboxLifecycle task can archive sandboxes that have been stopped
// for more than 24 hours.
func (o *Orchestrator) StopSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.provider.StopSandbox(ctx, sb.ExternalID); err != nil {
		return fmt.Errorf("stopping sandbox %s: %w", sb.ID, err)
	}
	now := time.Now()
	if err := o.db.Model(sb).Updates(map[string]any{
		"status":                "stopped",
		"stopped_at":            now,
		"bridge_url_expires_at": nil,
	}).Error; err != nil {
		return err
	}
	sb.Status = "stopped"
	sb.StoppedAt = &now
	sb.BridgeURLExpiresAt = nil
	return nil
}

// DeleteSandbox tears down a sandbox via the provider and removes the DB record.
func (o *Orchestrator) DeleteSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.provider.DeleteSandbox(ctx, sb.ExternalID); err != nil {
		slog.Warn("failed to delete sandbox from provider", "sandbox_id", sb.ID, "external_id", sb.ExternalID, "error", err)
		// Continue to delete DB record even if provider fails
	}
	return o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error
}

// StartHealthChecker runs a background goroutine that periodically syncs sandbox
// status from the provider and auto-stops idle sandboxes.
func (o *Orchestrator) StartHealthChecker(ctx context.Context) {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("sandbox health checker stopped")
			return
		case <-ticker.C:
			o.RunHealthCheck(ctx)
		}
	}
}

// --- internal helpers ---

// createSandbox creates a dedicated sandbox for an agent.
// Pool/shared sandboxes use createPoolSandbox instead.
func (o *Orchestrator) createSandbox(ctx context.Context, org *model.Org, agent *model.Agent) (*model.Sandbox, error) {
	// Ensure Turso storage for the org (optional — Bridge works without it)
	var storageURL, authToken string
	if o.turso != nil {
		var err error
		storageURL, authToken, err = o.turso.EnsureStorage(ctx, org.ID)
		if err != nil {
			slog.Warn("turso storage provisioning failed, continuing without libsql", "error", err)
		}
	}

	// Generate and encrypt Bridge API key
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

	// Build env vars for Bridge
	webhookURL := fmt.Sprintf("https://%s/internal/webhooks/bridge/%s", o.cfg.BridgeHost, sb.ID)
	envVars := baseEnvVars(o.cfg, bridgeAPIKey, sb.ID, webhookURL)
	setOrgEnvVars(envVars, org.ID)
	setAgentEnvVars(envVars, agent, o.cfg)
	setDriveEndpoint(envVars, sb.ID, o.cfg)
	if storageURL != "" {
		envVars["BRIDGE_STORAGE_URL"] = storageURL
		envVars["BRIDGE_STORAGE_AUTH_TOKEN"] = authToken
	}

	// Merge agent-level env vars for dedicated sandboxes
	if agent != nil {
		o.mergeUserEnvVars(envVars, agent.EncryptedEnvVars)
	}

	// Resolve snapshot
	snapshotID := o.resolveSnapshot(agent)

	// Build sandbox name
	name := o.buildSandboxName(agent)

	// Build labels
	labels := map[string]string{
		"org_id":       org.ID.String(),
		"sandbox_type": "dedicated",
		"sandbox_id":   sb.ID.String(),
	}
	if agent != nil {
		labels["agent_id"] = agent.ID.String()
	}

	// Create via provider (synchronous — blocks until running)
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

	// Ensure Bridge storage directory exists (snapshot may not have it)
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

	// Disable Daytona's auto-stop AND auto-archive. Sandbox lifecycle is
	// managed by our own background tasks — the periodic SandboxLifecycle
	// task stops idle sandboxes after 10m and archives stopped sandboxes
	// after 24h. intervalMinutes=0 disables each policy.
	disableProviderLifecycle(ctx, o.provider, &sb, info.ExternalID)

	// Run agent-level setup commands for dedicated sandboxes
	if agent != nil && len(agent.SetupCommands) > 0 {
		if err := o.runSetupCommands(ctx, &sb, agent.SetupCommands); err != nil {
			o.db.Model(&sb).Updates(map[string]any{
				"status":        "error",
				"error_message": fmt.Sprintf("setup commands failed: %v", err),
			})
			return nil, fmt.Errorf("setup commands failed: %w", err)
		}
	}

	// Clone configured repositories into the sandbox
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

// WakeSandbox starts a stopped (or archived) sandbox. Daytona uses the same
// StartSandbox endpoint for both resume-from-stop and restore-from-archive.
// Clears StoppedAt on success so the 24h archive timer resets.
func (o *Orchestrator) WakeSandbox(ctx context.Context, sb *model.Sandbox) (*model.Sandbox, error) {
	if err := o.provider.StartSandbox(ctx, sb.ExternalID); err != nil {
		return nil, fmt.Errorf("starting sandbox %s: %w", sb.ID, err)
	}

	if err := o.refreshBridgeURL(ctx, sb); err != nil {
		return nil, fmt.Errorf("refreshing bridge URL after wake: %w", err)
	}

	now := time.Now()
	o.db.Model(sb).Updates(map[string]any{
		"status":         "running",
		"last_active_at": now,
		"stopped_at":     nil,
		"error_message":  nil,
	})
	sb.Status = "running"
	sb.LastActiveAt = &now
	sb.StoppedAt = nil

	// Wait for Bridge to become healthy (it restarts automatically via entrypoint)
	if err := o.waitForBridgeHealthy(ctx, sb); err != nil {
		o.db.Model(sb).Updates(map[string]any{
			"status":        "error",
			"error_message": fmt.Sprintf("bridge not healthy after wake: %v", err),
		})
		return nil, fmt.Errorf("bridge not healthy after wake: %w", err)
	}

	slog.Info("sandbox woken", "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	return sb, nil
}

func (o *Orchestrator) needsURLRefresh(sb *model.Sandbox) bool {
	if sb.BridgeURL == "" {
		return true
	}
	if sb.BridgeURLExpiresAt == nil {
		return true
	}
	return time.Now().Add(bridgeURLRefreshBuffer).After(*sb.BridgeURLExpiresAt)
}

func (o *Orchestrator) refreshBridgeURL(ctx context.Context, sb *model.Sandbox) error {
	url, err := o.provider.GetEndpoint(ctx, sb.ExternalID, BridgePort)
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(bridgeURLTTL)
	if err := o.db.Model(sb).Updates(map[string]any{
		"bridge_url":            url,
		"bridge_url_expires_at": expiresAt,
	}).Error; err != nil {
		return fmt.Errorf("updating bridge URL: %w", err)
	}
	sb.BridgeURL = url
	sb.BridgeURLExpiresAt = &expiresAt
	return nil
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
		return fmt.Sprintf("zira-ded-%s-%s-%d", safeName, shortID(agent.ID), ts)
	}
	return fmt.Sprintf("zira-ded-%d", ts)
}

// RunHealthCheck syncs sandbox status from the provider and auto-stops idle sandboxes.
func (o *Orchestrator) RunHealthCheck(ctx context.Context) {
	var sandboxes []model.Sandbox
	if err := o.db.Where("status = 'running'").Find(&sandboxes).Error; err != nil {
		slog.Error("health check: failed to query sandboxes", "error", err)
		return
	}

	for i := range sandboxes {
		sb := &sandboxes[i]
		o.checkSandboxHealth(ctx, sb)
	}
}

func (o *Orchestrator) checkSandboxHealth(ctx context.Context, sb *model.Sandbox) {
	// Sync status from provider. Failures here are almost entirely provider-side
	// 404s for sandboxes that were deleted out-of-band — logging them on every
	// tick floods the asynq worker logs and drowns out real activity. Skip
	// silently; downstream lifecycle tasks will reconcile the DB state.
	status, err := o.provider.GetStatus(ctx, sb.ExternalID)
	if err != nil {
		return
	}

	providerStatus := string(status)
	if providerStatus != sb.Status {
		slog.Info("health check: status changed", "sandbox_id", sb.ID, "old", sb.Status, "new", providerStatus)
		o.db.Model(sb).Update("status", providerStatus)
		sb.Status = providerStatus
	}

	// Handle shared sandbox errors — unassign all agents
	if sb.Status == "error" && sb.SandboxType == "shared" {
		o.handleSharedSandboxError(sb)
		return
	}

	// System sandbox: never auto-stop, never sleep. If it's not running,
	// try to wake it; the periodic SystemAgentSync task will re-push the
	// agent definitions to Bridge after wake.
	if sb.SandboxType == "system" {
		if sb.Status == "stopped" {
			slog.Warn("system sandbox is stopped, attempting wake", "sandbox_id", sb.ID)
			if _, err := o.WakeSandbox(ctx, sb); err != nil {
				slog.Error("failed to wake system sandbox", "sandbox_id", sb.ID, "error", err)
			}
		}
		return
	}

	if sb.Status != "running" || sb.LastActiveAt == nil {
		return
	}

	idleMinutes := time.Since(*sb.LastActiveAt).Minutes()

	if sb.SandboxType == "shared" {
		// Shared sandboxes: only auto-stop if 0 agents and idle past threshold
		var agentCount int64
		o.db.Model(&model.Agent{}).Where("sandbox_id = ?", sb.ID).Count(&agentCount)
		if agentCount > 0 {
			return
		}
		threshold := o.cfg.PoolSandboxIdleTimeoutMins
		if threshold > 0 && int(idleMinutes) >= threshold {
			slog.Info("health check: auto-stopping empty shared sandbox",
				"sandbox_id", sb.ID, "idle_mins", int(idleMinutes))
			if err := o.StopSandbox(ctx, sb); err != nil {
				slog.Error("health check: failed to stop shared sandbox", "sandbox_id", sb.ID, "error", err)
			}
		}
	} else {
		// Dedicated sandboxes: lifecycle managed by background tasks, not health check.
		// Auto-stop is disabled via Daytona's SetAutoStop(0) at creation time.
	}
}

// handleSharedSandboxError unassigns all agents from an errored shared sandbox.
func (o *Orchestrator) handleSharedSandboxError(sb *model.Sandbox) {
	result := o.db.Model(&model.Agent{}).
		Where("sandbox_id = ?", sb.ID).
		Update("sandbox_id", nil)
	if result.RowsAffected > 0 {
		slog.Warn("unassigned agents from errored shared sandbox",
			"sandbox_id", sb.ID, "agents_affected", result.RowsAffected)
	}
}

// waitForBridgeHealthy polls Bridge's /health endpoint until it responds 200 or timeout.
func (o *Orchestrator) waitForBridgeHealthy(ctx context.Context, sb *model.Sandbox) error {
	healthURL := sb.BridgeURL + "/health"
	deadline := time.Now().Add(bridgeHealthTimeout)
	client := &http.Client{Timeout: 5 * time.Second}
	attempt := 0

	slog.Info("waiting for bridge healthy",
		"sandbox_id", sb.ID,
		"health_url", healthURL,
		"bridge_url", sb.BridgeURL,
	)

	for time.Now().Before(deadline) {
		attempt++

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return fmt.Errorf("creating health request: %w", err)
		}

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				slog.Info("bridge healthy",
					"sandbox_id", sb.ID,
					"attempts", attempt,
					"elapsed", time.Since(deadline.Add(-bridgeHealthTimeout)).String(),
				)
				return nil
			}
			slog.Info("bridge health check: non-200", "status", resp.StatusCode, "attempt", attempt, "url", healthURL)
		} else {
			slog.Info("bridge health check: connection failed", "attempt", attempt, "url", healthURL, "error", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(bridgeHealthInterval):
		}
	}

	return fmt.Errorf("bridge did not become healthy within %s (%d attempts)", bridgeHealthTimeout, attempt)
}

// ExecuteCommand runs a command inside a sandbox via the provider.
func (o *Orchestrator) ExecuteCommand(ctx context.Context, sb *model.Sandbox, command string) (string, error) {
	return o.provider.ExecuteCommand(ctx, sb.ExternalID, command)
}

// --- lifecycle management (sleep + archive) ---

const (
	// sandboxIdleTimeoutMinutes is how long a running sandbox can sit idle
	// (no bridge activity) before the periodic lifecycle task stops it.
	sandboxIdleTimeoutMinutes = 10

	// sandboxArchiveAfterHours is how long a stopped sandbox stays stopped
	// before the lifecycle task moves it to cold storage (archived).
	sandboxArchiveAfterHours = 24
)

// disableProviderLifecycle turns off the provider's auto-stop and auto-archive
// policies for a sandbox. Lifecycle is managed by the periodic
// SandboxLifecycle task — see RunSandboxLifecycle. Failures are logged but
// non-fatal; the lifecycle task is the source of truth and will stop/archive
// sandboxes independently of provider settings.
func disableProviderLifecycle(ctx context.Context, provider Provider, sb *model.Sandbox, externalID string) {
	if err := provider.SetAutoStop(ctx, externalID, 0); err != nil {
		slog.Warn("failed to disable provider auto-stop",
			"sandbox_id", sb.ID, "external_id", externalID, "error", err)
	}
	if err := provider.SetAutoArchive(ctx, externalID, 0); err != nil {
		slog.Warn("failed to disable provider auto-archive",
			"sandbox_id", sb.ID, "external_id", externalID, "error", err)
	}
}

// ArchiveSandbox moves a stopped sandbox into cold storage. The sandbox must
// already be stopped — the provider rejects archive on running sandboxes.
// On success, the DB row transitions to status='archived'. Use
// UnarchiveSandbox (or EnsureSandboxActive) to restore it later.
func (o *Orchestrator) ArchiveSandbox(ctx context.Context, sb *model.Sandbox) error {
	// Belt-and-braces: if the sandbox isn't stopped, stop it first.
	if sb.Status != string(StatusStopped) {
		if err := o.StopSandbox(ctx, sb); err != nil {
			return fmt.Errorf("stopping sandbox before archive: %w", err)
		}
	}

	if err := o.provider.ArchiveSandbox(ctx, sb.ExternalID); err != nil {
		return fmt.Errorf("archiving sandbox %s: %w", sb.ID, err)
	}

	if err := o.db.Model(sb).Updates(map[string]any{
		"status":                string(StatusArchived),
		"bridge_url_expires_at": nil,
	}).Error; err != nil {
		return fmt.Errorf("marking sandbox archived: %w", err)
	}
	sb.Status = string(StatusArchived)
	sb.BridgeURLExpiresAt = nil

	slog.Info("sandbox archived", "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	return nil
}

// UnarchiveSandbox restores an archived sandbox back to running state.
// Daytona uses the same StartSandbox call for waking stopped AND unarchiving
// archived sandboxes — but unarchive may take longer (restoring from cold
// storage), so we widen the bridge health deadline.
func (o *Orchestrator) UnarchiveSandbox(ctx context.Context, sb *model.Sandbox) (*model.Sandbox, error) {
	slog.Info("unarchiving sandbox", "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	// Optimistically mark as starting so concurrent callers don't re-trigger.
	o.db.Model(sb).Update("status", string(StatusStarting))
	sb.Status = string(StatusStarting)

	return o.WakeSandbox(ctx, sb)
}

// EnsureSandboxActive guarantees that a sandbox is in running state before
// downstream code tries to talk to it over Bridge. This is the single
// chokepoint called by every Bridge-using path:
//   - already running → no-op
//   - starting/creating → wait for bridge healthy
//   - stopped → wake
//   - archived → unarchive (which also wakes)
//   - error/unknown → surface the error
//
// It mutates sb in place with the refreshed fields (Status, BridgeURL,
// BridgeURLExpiresAt, LastActiveAt) so callers can keep using the same
// pointer without re-reading from DB.
func (o *Orchestrator) EnsureSandboxActive(ctx context.Context, sb *model.Sandbox) (*model.Sandbox, error) {
	switch sb.Status {
	case string(StatusRunning):
		return sb, nil

	case string(StatusStopped):
		return o.WakeSandbox(ctx, sb)

	case string(StatusArchived), string(StatusArchiving):
		return o.UnarchiveSandbox(ctx, sb)

	case string(StatusCreating), string(StatusStarting):
		// Provisioning in flight — wait for bridge.
		if err := o.waitForBridgeHealthy(ctx, sb); err != nil {
			return nil, fmt.Errorf("waiting for in-flight sandbox: %w", err)
		}
		now := time.Now()
		o.db.Model(sb).Updates(map[string]any{
			"status":         "running",
			"last_active_at": now,
		})
		sb.Status = "running"
		sb.LastActiveAt = &now
		return sb, nil

	case string(StatusError):
		return nil, fmt.Errorf("sandbox %s is in error state", sb.ID)

	default:
		// Unknown DB status — ask the provider for ground truth and retry.
		status, err := o.provider.GetStatus(ctx, sb.ExternalID)
		if err != nil {
			return nil, fmt.Errorf("getting provider status for sandbox %s: %w", sb.ID, err)
		}
		sb.Status = string(status)
		o.db.Model(sb).Update("status", sb.Status)
		if sb.Status == string(StatusRunning) {
			return sb, nil
		}
		// Recurse once with the reconciled status.
		return o.EnsureSandboxActive(ctx, sb)
	}
}

// RunSandboxLifecycle implements the periodic lifecycle policy for sandboxes:
//  1. Running sandboxes idle for >10 minutes → stop.
//  2. Stopped sandboxes stopped for >24 hours → archive.
//
// System sandboxes are exempt from both policies (they must stay running).
// Called from the periodic SandboxLifecycle asynq task every 5 minutes.
func (o *Orchestrator) RunSandboxLifecycle(ctx context.Context) {
	now := time.Now()
	idleCutoff := now.Add(-time.Duration(sandboxIdleTimeoutMinutes) * time.Minute)
	archiveCutoff := now.Add(-time.Duration(sandboxArchiveAfterHours) * time.Hour)

	// 1. Stop running sandboxes that have been idle past the idle threshold.
	var idleRunning []model.Sandbox
	if err := o.db.Where(
		"status = ? AND sandbox_type != ? AND last_active_at IS NOT NULL AND last_active_at < ?",
		string(StatusRunning),
		"system",
		idleCutoff,
	).Find(&idleRunning).Error; err != nil {
		slog.Error("sandbox lifecycle: query idle running sandboxes failed", "error", err)
	} else {
		for i := range idleRunning {
			sb := &idleRunning[i]
			// For shared sandboxes, skip if agents are still assigned.
			if sb.SandboxType == "shared" {
				var agentCount int64
				o.db.Model(&model.Agent{}).Where("sandbox_id = ?", sb.ID).Count(&agentCount)
				if agentCount > 0 {
					continue
				}
			}
			slog.Info("sandbox lifecycle: stopping idle sandbox",
				"sandbox_id", sb.ID,
				"external_id", sb.ExternalID,
				"idle_minutes", int(now.Sub(*sb.LastActiveAt).Minutes()),
			)
			if err := o.StopSandbox(ctx, sb); err != nil {
				slog.Error("sandbox lifecycle: failed to stop idle sandbox",
					"sandbox_id", sb.ID, "error", err)
			}
		}
	}

	// 2. Archive stopped sandboxes that have been stopped past the archive threshold.
	var staleStopped []model.Sandbox
	if err := o.db.Where(
		"status = ? AND sandbox_type != ? AND stopped_at IS NOT NULL AND stopped_at < ?",
		string(StatusStopped),
		"system",
		archiveCutoff,
	).Find(&staleStopped).Error; err != nil {
		slog.Error("sandbox lifecycle: query stale stopped sandboxes failed", "error", err)
	} else {
		for i := range staleStopped {
			sb := &staleStopped[i]
			slog.Info("sandbox lifecycle: archiving stale stopped sandbox",
				"sandbox_id", sb.ID,
				"external_id", sb.ExternalID,
				"stopped_hours", int(now.Sub(*sb.StoppedAt).Hours()),
			)
			if err := o.ArchiveSandbox(ctx, sb); err != nil {
				slog.Error("sandbox lifecycle: failed to archive stopped sandbox",
					"sandbox_id", sb.ID, "error", err)
			}
		}
	}
}

// resolveBuildOpts resolves the base image and resource allocation for a template build.
func (o *Orchestrator) resolveBuildOpts(tmpl *model.SandboxTemplate, snapshotName string) BuildSnapshotOpts {
	cmds := []string{}
	if tmpl.BuildCommands != "" {
		cmds = strings.Split(tmpl.BuildCommands, "\n")
	}

	opts := BuildSnapshotOpts{
		Name:          snapshotName,
		BuildCommands: cmds,
	}

	// Resolve base image from parent public template
	if tmpl.BaseTemplateID != nil {
		var baseTmpl model.SandboxTemplate
		if err := o.db.First(&baseTmpl, "id = ?", *tmpl.BaseTemplateID).Error; err == nil {
			if baseTmpl.ExternalID != nil {
				opts.BaseImage = *baseTmpl.ExternalID
			}
		}
	}

	// Resolve resource allocation from template size
	if sz, ok := model.TemplateSizes[tmpl.Size]; ok {
		opts.CPU = sz.CPU
		opts.Memory = sz.Memory
		opts.Disk = sz.Disk
	}

	return opts
}

// BuildTemplate builds a sandbox template (snapshot) via the provider.
// Runs asynchronously — updates the template record with build status.
func (o *Orchestrator) BuildTemplate(ctx context.Context, tmpl *model.SandboxTemplate) {
	o.db.Model(tmpl).Update("build_status", "building")

	opts := o.resolveBuildOpts(tmpl, tmpl.Slug)
	externalID, err := o.provider.BuildSnapshot(ctx, opts)

	if err != nil {
		errMsg := err.Error()
		o.db.Model(tmpl).Updates(map[string]any{
			"build_status": "failed",
			"build_error":  errMsg,
		})
		slog.Error("template build failed", "template_id", tmpl.ID, "error", err)
		return
	}

	o.db.Model(tmpl).Updates(map[string]any{
		"build_status": "ready",
		"external_id":  externalID,
		"build_error":  nil,
	})
	slog.Info("template built", "template_id", tmpl.ID, "external_id", externalID)
}

// BuildTemplateWithLogs builds a sandbox template and streams logs via onLog callback.
// Returns the external snapshot ID once the build completes.
func (o *Orchestrator) BuildTemplateWithLogs(ctx context.Context, tmpl *model.SandboxTemplate, onLog func(string)) (string, error) {
	opts := o.resolveBuildOpts(tmpl, tmpl.Slug)
	return o.provider.BuildSnapshotWithLogs(ctx, opts, onLog)
}

// BuildTemplateWithPolling builds a sandbox template, polls for status, and accumulates logs to DB.
// This is the recommended way to build templates as it properly handles async builds.
// onStatus is called whenever the build status changes (building, ready, failed).
func (o *Orchestrator) BuildTemplateWithPolling(ctx context.Context, tmpl *model.SandboxTemplate, onLog func(string), onStatus func(status, message string)) (externalID string, buildErr error) {
	opts := o.resolveBuildOpts(tmpl, tmpl.Slug)

	// Start the async build
	externalID, err := o.provider.BuildSnapshotWithLogs(ctx, opts, onLog)
	if err != nil {
		return "", fmt.Errorf("starting snapshot build: %w", err)
	}

	// Poll for snapshot status until ready or error (max 15 minutes)
	const pollInterval = 5 * time.Second
	const maxWait = 15 * time.Minute
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return externalID, ctx.Err()
		case <-time.After(pollInterval):
		}

		status, err := o.provider.GetSnapshotStatus(ctx, externalID)
		if err != nil {
			slog.Warn("failed to get snapshot status, retrying", "external_id", externalID, "error", err)
			continue
		}

		switch status.State {
		case "ready":
			slog.Info("snapshot build completed", "external_id", externalID)
			if onStatus != nil {
				onStatus("ready", "")
			}
			return externalID, nil
		case "error":
			errMsg := status.ErrorMsg
			if errMsg == "" {
				errMsg = "snapshot build failed with unknown error"
			}
			// Try to get logs from Daytona and append error reason
			if logs, logErr := o.provider.GetSnapshotLogs(ctx, externalID); logErr == nil && logs != "" {
				if onLog != nil {
					onLog(logs)
				}
			}
			// Append error reason to logs if available
			if status.ErrorReason != "" {
				errorLog := fmt.Sprintf("\n[ERROR REASON]: %s", status.ErrorReason)
				if onLog != nil {
					onLog(errorLog)
				}
				errMsg = fmt.Sprintf("%s\n%s", errMsg, status.ErrorReason)
			}
			slog.Error("snapshot build failed", "external_id", externalID, "error", errMsg)
			if onStatus != nil {
				onStatus("failed", errMsg)
			}
			return externalID, fmt.Errorf("%s", errMsg)
		case "building", "pending", "":
			// Continue polling
			slog.Debug("snapshot still building", "external_id", externalID, "state", status.State)
		default:
			slog.Warn("unknown snapshot state", "external_id", externalID, "state", status.State)
		}
	}

	return externalID, fmt.Errorf("snapshot build timed out after %s", maxWait)
}

// DeleteTemplate deletes a sandbox template (snapshot) from the provider.
func (o *Orchestrator) DeleteTemplate(ctx context.Context, externalID string) error {
	return o.provider.DeleteSnapshot(ctx, externalID)
}

// RetryTemplateBuild deletes an existing snapshot and starts a new build.
// If newCommands is provided, updates the template with those commands first.
func (o *Orchestrator) RetryTemplateBuild(ctx context.Context, tmpl *model.SandboxTemplate, newCommands []string, onLog func(string), onStatus func(status, message string)) (externalID string, buildErr error) {
	// Delete existing snapshot if present
	if tmpl.ExternalID != nil && *tmpl.ExternalID != "" {
		slog.Info("deleting existing snapshot before retry", "external_id", *tmpl.ExternalID)
		if err := o.provider.DeleteSnapshot(ctx, *tmpl.ExternalID); err != nil {
			slog.Warn("failed to delete existing snapshot", "external_id", *tmpl.ExternalID, "error", err)
		}
	}

	// Update commands if provided
	if len(newCommands) > 0 {
		tmpl.BuildCommands = strings.Join(newCommands, "\n")
	}

	// Reset template status
	tmpl.ExternalID = nil
	tmpl.BuildStatus = "building"
	tmpl.BuildError = nil
	tmpl.BuildLogs = ""

	// Build with polling
	return o.BuildTemplateWithPolling(ctx, tmpl, onLog, onStatus)
}

// mergeUserEnvVars decrypts and merges user-defined env vars into the system env vars map.
// System vars (BRIDGE_*) are never overridden.
func (o *Orchestrator) mergeUserEnvVars(envVars map[string]string, encrypted []byte) {
	if o.encKey == nil || len(encrypted) == 0 {
		return
	}
	decrypted, err := o.encKey.DecryptString(encrypted)
	if err != nil {
		slog.Warn("failed to decrypt user env vars, skipping", "error", err)
		return
	}
	var userVars map[string]string
	if err := json.Unmarshal([]byte(decrypted), &userVars); err != nil {
		slog.Warn("failed to parse user env vars, skipping", "error", err)
		return
	}
	for k, v := range userVars {
		// Never override system vars
		if strings.HasPrefix(strings.ToUpper(k), "BRIDGE_") {
			continue
		}
		envVars[k] = v
	}
}

// repoResource is a single repository from the agent's resources config.
type repoResource struct {
	ID   string `json:"id"`   // e.g. "owner/repo-name"
	Name string `json:"name"` // e.g. "repo-name"
}

// cloneAgentRepositories parses the agent's configured resources and clones any
// github-app repositories into /home/daytona/repos/{name}.
func (o *Orchestrator) cloneAgentRepositories(ctx context.Context, sb *model.Sandbox, agent *model.Agent) error {
	if agent.Resources == nil || len(agent.Resources) == 0 {
		return nil
	}

	// Collect repos from all connections
	var repos []repoResource
	for _, resourceTypes := range agent.Resources {
		typesMap, ok := resourceTypes.(map[string]any)
		if !ok {
			continue
		}
		repoList, ok := typesMap["repository"]
		if !ok {
			continue
		}
		repoSlice, ok := repoList.([]any)
		if !ok {
			continue
		}
		for _, item := range repoSlice {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			repoID, _ := itemMap["id"].(string)
			repoName, _ := itemMap["name"].(string)
			if repoID != "" && repoName != "" {
				repos = append(repos, repoResource{ID: repoID, Name: repoName})
			}
		}
	}

	if len(repos) == 0 {
		return nil
	}

	// Create the repos directory
	if _, err := o.ExecuteCommand(ctx, sb, "mkdir -p /home/daytona/repos"); err != nil {
		return fmt.Errorf("creating repos directory: %w", err)
	}

	for _, repo := range repos {
		repoPath := "/home/daytona/repos/" + repo.Name
		cloneURL := "https://github.com/" + repo.ID + ".git"

		slog.Info("cloning repository into sandbox",
			"sandbox_id", sb.ID,
			"repo", repo.ID,
			"path", repoPath,
		)

		// Clone — git credential helper handles auth automatically
		output, err := o.ExecuteCommand(ctx, sb,
			fmt.Sprintf("git clone --depth=1 %s %s", cloneURL, repoPath))
		if err != nil {
			slog.Error("git clone failed",
				"sandbox_id", sb.ID,
				"repo", repo.ID,
				"output", output,
				"error", err,
			)
			return fmt.Errorf("cloning %s: %w", repo.ID, err)
		}

		slog.Info("repository cloned",
			"sandbox_id", sb.ID,
			"repo", repo.ID,
			"path", repoPath,
		)
	}

	return nil
}

// runSetupCommands executes a list of shell commands inside the sandbox sequentially.
func (o *Orchestrator) runSetupCommands(ctx context.Context, sb *model.Sandbox, commands []string) error {
	for _, cmd := range commands {
		output, err := o.ExecuteCommand(ctx, sb, cmd)
		if err != nil {
			slog.Error("setup command failed",
				"sandbox_id", sb.ID,
				"command", cmd,
				"output", output,
				"error", err,
			)
			return fmt.Errorf("setup command failed: %s: %w", cmd, err)
		}
		slog.Info("setup command completed",
			"sandbox_id", sb.ID,
			"command", cmd,
		)
	}
	return nil
}

// --- utilities ---

func generateRandomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func shortID(id uuid.UUID) string {
	return strings.ReplaceAll(id.String(), "-", "")[:12]
}

func sanitizeName(name string) string {
	// Keep only alphanumeric + hyphens, lowercase, truncate
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) > 20 {
		s = s[:20]
	}
	return s
}
