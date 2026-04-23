package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/crypto"
	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/turso"
)

// Orchestrator manages sandbox lifecycle — creating, starting, stopping sandboxes
// and providing BridgeClients to talk to them.
type Orchestrator struct {
	db       *gorm.DB
	provider Provider
	turso    *turso.Provisioner
	encKey   *crypto.SymmetricKey
	cfg      *config.Config
}

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
	if agent.SandboxID != nil {
		var existing model.Sandbox
		if err := o.db.Where("id = ?", *agent.SandboxID).First(&existing).Error; err == nil {
			if err := o.verifySandboxExists(ctx, &existing); err == nil {
				switch existing.Status {
				case "running":
					return &existing, nil
				default:
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
		o.db.Model(agent).Update("sandbox_id", nil)
		agent.SandboxID = nil
	}

	threshold := o.cfg.PoolSandboxResourceThreshold
	var sb model.Sandbox
	err := o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(`
			SELECT * FROM sandboxes
			WHERE sandbox_type = 'shared'
			  AND status = 'running'
			  AND (memory_limit_bytes = 0 OR (memory_used_bytes * 100.0 / memory_limit_bytes) < ?)
			ORDER BY CASE WHEN memory_limit_bytes = 0 THEN 0 ELSE (memory_used_bytes * 100.0 / memory_limit_bytes) END ASC,
			         id ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		`, threshold).Scan(&sb).Error; err != nil {
			return err
		}

		if sb.ID == uuid.Nil {
			return gorm.ErrRecordNotFound
		}

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

	newSb, err := o.createPoolSandbox(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating shared sandbox on demand: %w", err)
	}

	o.db.Model(agent).Update("sandbox_id", newSb.ID)
	agent.SandboxID = &newSb.ID

	return newSb, nil
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

	if err := o.db.Model(&model.Agent{}).
		Where("is_system = true").
		Update("sandbox_id", sb.ID).Error; err != nil {
		return nil, fmt.Errorf("binding system agents to sandbox: %w", err)
	}

	return &sb, nil
}

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
	apiKey, err := o.encKey.DecryptString(sb.EncryptedBridgeAPIKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting bridge api key: %w", err)
	}

	if _, err := o.EnsureSandboxActive(ctx, sb); err != nil {
		return nil, fmt.Errorf("ensuring sandbox active: %w", err)
	}

	if o.needsURLRefresh(sb) {
		if err := o.refreshBridgeURL(ctx, sb); err != nil {
			return nil, fmt.Errorf("refreshing bridge URL: %w", err)
		}
	}

	o.touchLastActive(sb)

	return bridge.NewBridgeClient(sb.BridgeURL, apiKey), nil
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
