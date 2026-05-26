package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// Orchestrator manages sandbox lifecycle — creating, starting, stopping sandboxes
// and providing runtime clients to talk to them.
type Orchestrator struct {
	db                *gorm.DB
	provider          Provider
	encKey            *crypto.SymmetricKey
	cfg               *config.Config
	warmPool          *WarmPool
	reconcileWarmPool func(context.Context, string, string) error
}

func NewOrchestrator(db *gorm.DB, provider Provider, encKey *crypto.SymmetricKey, cfg *config.Config) *Orchestrator {
	return &Orchestrator{
		db:       db,
		provider: provider,
		encKey:   encKey,
		cfg:      cfg,
		warmPool: NewWarmPool(db, provider, encKey, cfg),
	}
}

func (o *Orchestrator) WarmPool() *WarmPool {
	return o.warmPool
}

func (o *Orchestrator) SetWarmPoolReconciler(fn func(context.Context, string, string) error) {
	o.reconcileWarmPool = fn
}

func (o *Orchestrator) enqueueWarmPoolReconcile(ctx context.Context, mode string) {
	if o.reconcileWarmPool == nil {
		return
	}
	if err := o.reconcileWarmPool(ctx, o.provider.ID(), mode); err != nil {
		logging.Capture(ctx, fmt.Errorf("enqueue warm pool reconcile: %w", err))
	}
}

func (o *Orchestrator) ProviderID() string {
	return o.providerID()
}

func (o *Orchestrator) GetRuntimeClient(ctx context.Context, sb *model.Sandbox) (*employeeruntime.Client, error) {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return nil, err
	}
	apiKey, err := o.encKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypting runtime secret: %w", err)
	}
	if _, err := o.EnsureSandboxActive(ctx, sb); err != nil {
		return nil, fmt.Errorf("ensuring sandbox active: %w", err)
	}
	if o.needsURLRefresh(sb) {
		if err := o.RefreshEmployeeSandboxURL(ctx, sb); err != nil {
			return nil, fmt.Errorf("refreshing runtime URL: %w", err)
		}
	}
	o.touchLastActive(sb)
	return employeeruntime.NewClient(sb.RuntimeURL, apiKey), nil
}

func (o *Orchestrator) CreateSpecialistSandbox(ctx context.Context, agent *model.Employee) (*model.Sandbox, error) {
	return o.CreateSpecialistSandboxWithEnv(ctx, agent, nil)
}

func (o *Orchestrator) CreateSpecialistSandboxWithEnv(ctx context.Context, agent *model.Employee, extraEnv map[string]string) (*model.Sandbox, error) {
	if agent.OrgID == nil {
		return nil, fmt.Errorf("cannot create specialist sandbox for agent without org_id")
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

// StartHealthChecker runs a background goroutine that periodically syncs sandbox
// status from the provider and auto-stops idle sandboxes.
func (o *Orchestrator) StartHealthChecker(ctx context.Context) {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logging.FromContext(ctx).InfoContext(ctx, "sandbox health checker stopped")
			return
		case <-ticker.C:
			o.RunHealthCheck(ctx)
		}
	}
}
