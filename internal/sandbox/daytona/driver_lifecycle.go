package daytona

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	toolbox "github.com/daytonaio/daytona/libs/toolbox-api-client-go"

	"github.com/usehiveloop/hiveloop/internal/sandbox"
)

const executeCommandTimeout = 120 * time.Second

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
	client, err := d.toolboxClient(externalID)
	if err != nil {
		return "", fmt.Errorf("building toolbox client for sandbox %s: %w", externalID, err)
	}

	execCtx, cancel := context.WithTimeout(ctx, executeCommandTimeout)
	defer cancel()

	req := toolbox.NewExecuteRequest(command)
	resp, httpResp, err := client.ProcessAPI.ExecuteCommand(execCtx).Request(*req).Execute()
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 404 {
			return "", sandbox.ErrSandboxNotFound
		}
		return "", fmt.Errorf("executing command on sandbox %s: %w", externalID, err)
	}
	exitCode := int(resp.GetExitCode())
	result := resp.GetResult()
	if exitCode != 0 {
		return result, fmt.Errorf("command exited with code %d on sandbox %s: %s", exitCode, externalID, result)
	}
	return result, nil
}

// SDK default reads sandbox.toolboxProxyUrl (preview.<host>, not in our DNS).
func (d *Driver) toolboxClient(externalID string) (*toolbox.APIClient, error) {
	parsed, err := url.Parse(d.apiURL)
	if err != nil {
		return nil, fmt.Errorf("parsing API URL %q: %w", d.apiURL, err)
	}
	basePath := strings.TrimRight(parsed.Path, "/") + "/toolbox/" + externalID + "/toolbox"
	cfg := toolbox.NewConfiguration()
	cfg.Host = parsed.Host
	cfg.Scheme = parsed.Scheme
	cfg.Servers = toolbox.ServerConfigurations{
		{URL: fmt.Sprintf("%s://%s%s", parsed.Scheme, parsed.Host, basePath)},
	}
	cfg.AddDefaultHeader("Authorization", "Bearer "+d.apiKey)
	return toolbox.NewAPIClient(cfg), nil
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
