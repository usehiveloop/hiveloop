package sandbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

var (
	warmRuntimeClaimMaxWait      = 2 * time.Minute
	warmRuntimeClaimInitialDelay = 500 * time.Millisecond
	warmRuntimeClaimMaxDelay     = 10 * time.Second
)

func (o *Orchestrator) claimWarmRuntime(ctx context.Context, sb *model.Sandbox, mode string) error {
	if o.warmPool == nil {
		return fmt.Errorf("railway warm pool is not configured")
	}
	claimed, err := o.claimWarmRuntimeSlot(ctx, mode, sb.ID)
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

func (o *Orchestrator) claimWarmRuntimeSlot(ctx context.Context, mode string, sandboxID uuid.UUID) (*ClaimedWarmSlot, error) {
	deadline := time.Now().Add(warmRuntimeClaimMaxWait)
	delay := warmRuntimeClaimInitialDelay
	var lastErr error
	for {
		claimed, err := o.warmPool.Claim(ctx, mode, sandboxID)
		if err == nil {
			return claimed, nil
		}
		lastErr = err
		if !isNoWarmSlotAvailable(err) {
			return nil, err
		}
		o.enqueueWarmPoolReconcile(ctx, mode)
		if time.Now().Add(delay).After(deadline) {
			return nil, fmt.Errorf("no warm %s runtime available after %s: %w", mode, warmRuntimeClaimMaxWait, lastErr)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for warm %s runtime: %w: %w", mode, ctx.Err(), lastErr)
		case <-time.After(delay):
		}
		delay *= 2
		if delay > warmRuntimeClaimMaxDelay {
			delay = warmRuntimeClaimMaxDelay
		}
	}
}

func isNoWarmSlotAvailable(err error) bool {
	if err == nil {
		return false
	}
	return err == gorm.ErrRecordNotFound ||
		(strings.Contains(err.Error(), "no warm ") && strings.Contains(err.Error(), "sandbox slots available"))
}
