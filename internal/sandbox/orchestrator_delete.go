package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/usehiveloop/hiveloop/internal/model"
)

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

func (o *Orchestrator) DeleteSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.provider.DeleteSandbox(ctx, sb.ExternalID); err != nil {
		slog.Warn("failed to delete sandbox from provider", "sandbox_id", sb.ID, "external_id", sb.ExternalID, "error", err)
	}
	return o.db.Where("id = ?", sb.ID).Delete(&model.Sandbox{}).Error
}

func (o *Orchestrator) ArchiveSandbox(ctx context.Context, sb *model.Sandbox) error {
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

func (o *Orchestrator) UnarchiveSandbox(ctx context.Context, sb *model.Sandbox) (*model.Sandbox, error) {
	slog.Info("unarchiving sandbox", "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	o.db.Model(sb).Update("status", string(StatusStarting))
	sb.Status = string(StatusStarting)

	return o.WakeSandbox(ctx, sb)
}
