package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func (o *Orchestrator) verifySandboxExists(ctx context.Context, sb *model.Sandbox) error {
	if sb.ExternalID == "" {
		return fmt.Errorf("no external ID")
	}
	_, err := o.provider.GetEndpoint(ctx, sb.ExternalID, BridgePort)
	return err
}

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

func (o *Orchestrator) ExecuteCommand(ctx context.Context, sb *model.Sandbox, command string) (string, error) {
	return o.provider.ExecuteCommand(ctx, sb.ExternalID, command)
}
