package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) StopSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.provider.StopSandbox(ctx, sb.ExternalID); err != nil {
		if errors.Is(err, ErrSandboxNotFound) {
			return o.purgeMissingSandbox(sb)
		}
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

func (o *Orchestrator) DeleteSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.provider.DeleteSandbox(ctx, sb.ExternalID); err != nil && !errors.Is(err, ErrSandboxNotFound) {
		logging.Capture(ctx, fmt.Errorf("delete sandbox %s from provider: %w", sb.ID, err))
	}
	return o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error
}

// DeleteSandboxResource deletes the provider resource but keeps the control
// plane sandbox row for task/session history that points at the sandbox.
func (o *Orchestrator) DeleteSandboxResource(ctx context.Context, sb *model.Sandbox) error {
	if err := o.provider.DeleteSandbox(ctx, sb.ExternalID); err != nil && !errors.Is(err, ErrSandboxNotFound) {
		logging.Capture(ctx, fmt.Errorf("delete sandbox %s from provider: %w", sb.ID, err))
		return fmt.Errorf("delete sandbox resource %s: %w", sb.ID, err)
	}
	now := time.Now()
	if err := o.db.Model(sb).Updates(map[string]any{
		"status":                string(StatusArchived),
		"stopped_at":            now,
		"bridge_url_expires_at": nil,
	}).Error; err != nil {
		return fmt.Errorf("mark sandbox resource deleted: %w", err)
	}
	sb.Status = string(StatusArchived)
	sb.StoppedAt = &now
	sb.BridgeURLExpiresAt = nil
	return nil
}

func (o *Orchestrator) DeleteSandboxExternal(ctx context.Context, externalID string) error {
	if err := o.provider.DeleteSandbox(ctx, externalID); err != nil && !errors.Is(err, ErrSandboxNotFound) {
		return fmt.Errorf("delete sandbox %s from provider: %w", externalID, err)
	}
	return nil
}

func (o *Orchestrator) ArchiveSandbox(ctx context.Context, sb *model.Sandbox) error {
	if sb.Status != string(StatusStopped) {
		if err := o.StopSandbox(ctx, sb); err != nil {
			if errors.Is(err, ErrSandboxNotFound) {
				return nil
			}
			return fmt.Errorf("stopping sandbox before archive: %w", err)
		}
	}

	if err := o.provider.ArchiveSandbox(ctx, sb.ExternalID); err != nil {
		if errors.Is(err, ErrSandboxNotFound) {
			return o.purgeMissingSandbox(sb)
		}
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

	logging.FromContext(ctx).InfoContext(ctx, "sandbox archived", "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	return nil
}

func (o *Orchestrator) purgeMissingSandbox(sb *model.Sandbox) error {
	if err := o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error; err != nil {
		return fmt.Errorf("purging missing sandbox %s: %w", sb.ID, err)
	}
	return ErrSandboxNotFound
}

func (o *Orchestrator) UnarchiveSandbox(ctx context.Context, sb *model.Sandbox) (*model.Sandbox, error) {
	o.db.Model(sb).Update("status", string(StatusStarting))
	sb.Status = string(StatusStarting)

	return o.WakeSandbox(ctx, sb)
}
