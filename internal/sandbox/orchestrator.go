package sandbox

import (
	"context"
	"fmt"

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

func (o *Orchestrator) CreateDedicatedSandbox(ctx context.Context, agent *model.Agent) (*model.Sandbox, error) {
	return o.CreateDedicatedSandboxWithEnv(ctx, agent, nil)
}

func (o *Orchestrator) CreateDedicatedSandboxWithEnv(ctx context.Context, agent *model.Agent, extraEnv map[string]string) (*model.Sandbox, error) {
	if agent.OrgID == nil {
		return nil, fmt.Errorf("cannot create dedicated sandbox for agent without org_id")
	}
	var org model.Org
	if err := o.db.Where("id = ?", *agent.OrgID).First(&org).Error; err != nil {
		return nil, fmt.Errorf("loading org: %w", err)
	}

	return o.createSandbox(ctx, &org, agent, extraEnv)
}

func (o *Orchestrator) EmployeeTaskDriveUploadURL(employeeID, taskID uuid.UUID) string {
	return employeeDriveUploadURL(o.cfg, employeeID, "tasks/"+taskID.String())
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
