package daytona

import (
	"context"
	"fmt"

	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

// SetAutoStop disables or sets the platform's auto-stop interval. The
// high-level pkg/daytona SDK only exposes auto-stop at create-time, so we
// reach into api-client-go for the post-create setter.
func (d *Driver) SetAutoStop(ctx context.Context, externalID string, intervalMinutes int) error {
	_, _, err := d.apiClient.SandboxAPI.
		SetAutostopInterval(d.authCtx(ctx), externalID, float32(intervalMinutes)).
		Execute()
	if err != nil {
		return fmt.Errorf("setting auto-stop on sandbox %s: %w", externalID, err)
	}
	return nil
}

func (d *Driver) SetAutoArchive(ctx context.Context, externalID string, intervalMinutes int) error {
	sb, err := d.sdk.Get(ctx, externalID)
	if err != nil {
		if isSDKNotFound(err) {
			return sandbox.ErrSandboxNotFound
		}
		return fmt.Errorf("getting sandbox %s: %w", externalID, err)
	}
	interval := intervalMinutes
	if err := sb.SetAutoArchiveInterval(ctx, &interval); err != nil {
		return fmt.Errorf("setting auto-archive on sandbox %s: %w", externalID, err)
	}
	return nil
}

func (d *Driver) ExecuteCommand(ctx context.Context, externalID string, command string) (string, error) {
	sb, err := d.sdk.Get(ctx, externalID)
	if err != nil {
		if isSDKNotFound(err) {
			return "", sandbox.ErrSandboxNotFound
		}
		return "", fmt.Errorf("getting sandbox %s: %w", externalID, err)
	}
	resp, err := sb.Process.ExecuteCommand(ctx, command)
	if err != nil {
		return "", fmt.Errorf("executing command on sandbox %s: %w", externalID, err)
	}
	if resp.ExitCode != 0 {
		return resp.Result, fmt.Errorf("command exited with code %d on sandbox %s: %s", resp.ExitCode, externalID, resp.Result)
	}
	return resp.Result, nil
}

// mapState normalizes Daytona's API state strings into our SandboxStatus enum.
// Daytona uses "started" where we use "running"; everything else lines up.
func mapState(state string) sandbox.SandboxStatus {
	switch state {
	case "started", "running":
		return sandbox.StatusRunning
	case "stopped":
		return sandbox.StatusStopped
	case "creating", "starting", "pending", "restoring":
		return sandbox.StatusStarting
	case "archived":
		return sandbox.StatusArchived
	case "archiving":
		return sandbox.StatusArchiving
	case "error", "unknown", "destroyed", "destroying":
		return sandbox.StatusError
	default:
		return sandbox.StatusError
	}
}
