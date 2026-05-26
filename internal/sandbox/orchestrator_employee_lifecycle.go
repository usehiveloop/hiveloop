package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) RefreshEmployeeSandboxURL(ctx context.Context, sb *model.Sandbox) error {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return err
	}
	url, err := o.provider.GetEndpoint(ctx, sb.ExternalID, EmployeeSandboxPort)
	if err != nil {
		return fmt.Errorf("get employee sandbox endpoint: %w", err)
	}
	expiresAt := time.Now().Add(runtimeURLTTL)
	if err := o.db.Model(sb).Updates(map[string]any{
		"runtime_url":            url,
		"runtime_url_expires_at": expiresAt,
	}).Error; err != nil {
		return fmt.Errorf("update sandbox url: %w", err)
	}
	sb.RuntimeURL = url
	sb.RuntimeURLExpiresAt = &expiresAt
	return nil
}

func (o *Orchestrator) StartEmployeeSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return err
	}
	if err := o.provider.StartSandbox(ctx, sb.ExternalID); err != nil {
		return fmt.Errorf("starting employee sandbox %s: %w", sb.ID, err)
	}
	if err := o.RefreshEmployeeSandboxURL(ctx, sb); err != nil {
		return err
	}
	now := time.Now()
	if err := o.db.Model(sb).Updates(map[string]any{
		"status":         string(StatusRunning),
		"last_active_at": now,
		"stopped_at":     nil,
		"error_message":  nil,
	}).Error; err != nil {
		return fmt.Errorf("mark employee sandbox running: %w", err)
	}
	sb.Status = string(StatusRunning)
	sb.LastActiveAt = &now
	sb.StoppedAt = nil
	sb.ErrorMessage = nil
	if err := o.waitForEmployeeRuntimeLive(ctx, sb); err != nil {
		return fmt.Errorf("waiting for employee runtime: %w", err)
	}
	return nil
}

func (o *Orchestrator) RestartEmployeeSandbox(ctx context.Context, sb *model.Sandbox) error {
	if err := o.StopSandbox(ctx, sb); err != nil {
		return err
	}
	return o.StartEmployeeSandbox(ctx, sb)
}

func (o *Orchestrator) NeedsURLRefresh(sb *model.Sandbox) bool {
	return o.needsURLRefresh(sb)
}
