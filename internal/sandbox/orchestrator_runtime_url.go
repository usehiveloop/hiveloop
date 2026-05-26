package sandbox

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (o *Orchestrator) touchLastActive(sb *model.Sandbox) {
	now := time.Now()
	sb.LastActiveAt = &now
	go func(id uuid.UUID) {
		_ = o.db.Model(&model.Sandbox{}).
			Where("id = ?", id).
			Update("last_active_at", now).Error
	}(sb.ID)
}

func (o *Orchestrator) needsURLRefresh(sb *model.Sandbox) bool {
	if sb.RuntimeURL == "" {
		return true
	}
	if sb.RuntimeURLExpiresAt == nil {
		return true
	}
	return time.Now().Add(runtimeURLRefreshBuffer).After(*sb.RuntimeURLExpiresAt)
}

func (o *Orchestrator) refreshRuntimeURL(ctx context.Context, sb *model.Sandbox) error {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return err
	}
	url, err := o.provider.GetEndpoint(ctx, sb.ExternalID, RuntimePort)
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(runtimeURLTTL)
	if err := o.db.Model(sb).Updates(map[string]any{
		"runtime_url":            url,
		"runtime_url_expires_at": expiresAt,
	}).Error; err != nil {
		return fmt.Errorf("updating runtime URL: %w", err)
	}
	sb.RuntimeURL = url
	sb.RuntimeURLExpiresAt = &expiresAt
	return nil
}

func (o *Orchestrator) waitForRuntimeHealthy(ctx context.Context, sb *model.Sandbox) error {
	healthURL := sb.RuntimeURL + "/health"
	deadline := time.Now().Add(runtimeHealthTimeout)
	client := &http.Client{Timeout: 5 * time.Second}
	attempt := 0

	logging.FromContext(ctx).InfoContext(ctx, "waiting for runtime healthy", "sandbox_id", sb.ID)

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
				logging.FromContext(ctx).InfoContext(ctx, "runtime healthy", "sandbox_id", sb.ID, "attempts", attempt)
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(runtimeHealthInterval):
		}
	}

	return fmt.Errorf("runtime did not become healthy within %s (%d attempts)", runtimeHealthTimeout, attempt)
}

func (o *Orchestrator) ExecuteCommand(ctx context.Context, sb *model.Sandbox, command string) (string, error) {
	return o.ExecuteCommandWithTimeout(ctx, sb, command, 0)
}

func (o *Orchestrator) ExecuteCommandWithTimeout(ctx context.Context, sb *model.Sandbox, command string, timeout time.Duration) (string, error) {
	if err := o.ensureSandboxProvider(sb); err != nil {
		return "", err
	}
	if o.provider.ID() == ProviderRailway {
		apiKey, err := o.encKey.DecryptString(sb.EncryptedRuntimeSecret)
		if err != nil {
			return "", fmt.Errorf("decrypt runtime secret: %w", err)
		}
		seconds := int(timeout.Seconds())
		if seconds <= 0 {
			seconds = 120
		}
		client := employeeruntime.NewClient(sb.RuntimeURL, apiKey)
		resp, err := client.RunCommands(ctx, employeeruntime.ControlCommandsRequest{
			Commands:       []string{command},
			TimeoutSeconds: seconds,
			StopOnError:    true,
		})
		if err != nil {
			return "", err
		}
		if len(resp.Results) == 0 {
			return "", fmt.Errorf("runtime command returned no results")
		}
		result := resp.Results[0]
		if !resp.OK {
			return result.Output, fmt.Errorf("command failed in runtime: exit_code=%v timed_out=%v", result.ExitCode, result.TimedOut)
		}
		return result.Output, nil
	}
	return o.provider.ExecuteCommandWithTimeout(ctx, sb.ExternalID, command, timeout)
}
