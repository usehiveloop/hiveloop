package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) claimWarmRuntime(ctx context.Context, sb *model.Sandbox, mode string) error {
	if o.warmPool == nil {
		return fmt.Errorf("railway warm pool is not configured")
	}
	claimed, err := o.warmPool.Claim(ctx, mode, sb.ID)
	if err != nil {
		return fmt.Errorf("claim warm runtime: %w", err)
	}

	encryptedRuntimeSecret, err := o.encKey.EncryptString(claimed.RuntimeSecret)
	if err != nil {
		_ = o.warmPool.MarkError(context.WithoutCancel(ctx), claimed.ID, fmt.Sprintf("encrypt runtime secret: %v", err))
		return fmt.Errorf("encrypt claimed runtime secret: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(runtimeURLTTL)
	if err := o.db.Model(sb).Updates(map[string]any{
		"external_id":              claimed.ExternalID,
		"runtime_url":              claimed.EndpointURL,
		"runtime_url_expires_at":   expiresAt,
		"encrypted_runtime_secret": encryptedRuntimeSecret,
		"status":                   "running",
		"last_active_at":           now,
	}).Error; err != nil {
		_ = o.warmPool.MarkError(context.WithoutCancel(ctx), claimed.ID, fmt.Sprintf("update sandbox: %v", err))
		return fmt.Errorf("updating claimed sandbox: %w", err)
	}
	sb.ExternalID = claimed.ExternalID
	sb.RuntimeURL = claimed.EndpointURL
	sb.RuntimeURLExpiresAt = &expiresAt
	sb.EncryptedRuntimeSecret = encryptedRuntimeSecret
	sb.Status = "running"
	sb.LastActiveAt = &now

	if err := o.waitForEmployeeRuntimeLive(ctx, sb); err != nil {
		_ = o.warmPool.MarkError(context.WithoutCancel(ctx), claimed.ID, fmt.Sprintf("runtime health: %v", err))
		return fmt.Errorf("waiting for claimed runtime: %w", err)
	}
	if err := o.warmPool.MarkClaimed(ctx, claimed.ID); err != nil {
		return fmt.Errorf("mark warm slot claimed: %w", err)
	}
	o.enqueueWarmPoolReconcile(ctx, mode)
	return nil
}
