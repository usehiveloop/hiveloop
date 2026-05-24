package daytona

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	toolbox "github.com/daytonaio/daytona/libs/toolbox-api-client-go"

	"github.com/usehivy/hivy/internal/sandbox"
)

const executeCommandTimeout = 120 * time.Second

const cgroupResourceCommand = `cat /sys/fs/cgroup/memory.max /sys/fs/cgroup/memory.current /sys/fs/cgroup/memory.peak && cat /sys/fs/cgroup/cpu.max && grep -E "usage_usec|nr_throttled" /sys/fs/cgroup/cpu.stat && cat /sys/fs/cgroup/pids.current`

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
	return d.ExecuteCommandWithTimeout(ctx, externalID, command, executeCommandTimeout)
}

func (d *Driver) ExecuteCommandWithTimeout(ctx context.Context, externalID string, command string, timeout time.Duration) (string, error) {
	client, err := d.toolboxClient(externalID)
	if err != nil {
		return "", fmt.Errorf("building toolbox client for sandbox %s: %w", externalID, err)
	}
	if timeout <= 0 {
		timeout = executeCommandTimeout
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
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

func (d *Driver) GetResourceUsage(ctx context.Context, externalID string) (*sandbox.ResourceUsage, error) {
	output, err := d.ExecuteCommand(ctx, externalID, cgroupResourceCommand)
	if err != nil {
		return nil, err
	}
	return parseCgroupResourceUsage(output)
}

func parseCgroupResourceUsage(output string) (*sandbox.ResourceUsage, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 7 {
		return nil, fmt.Errorf("expected 7 lines, got %d", len(lines))
	}

	stats := &sandbox.ResourceUsage{CPUQuota: lines[3]}
	var err error
	if lines[0] != "max" {
		stats.MemoryLimitBytes, err = strconv.ParseInt(lines[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing memory.max %q: %w", lines[0], err)
		}
	}
	if stats.MemoryUsedBytes, err = strconv.ParseInt(lines[1], 10, 64); err != nil {
		return nil, fmt.Errorf("parsing memory.current %q: %w", lines[1], err)
	}
	if stats.MemoryPeakBytes, err = strconv.ParseInt(lines[2], 10, 64); err != nil {
		return nil, fmt.Errorf("parsing memory.peak %q: %w", lines[2], err)
	}
	if parts := strings.Fields(lines[4]); len(parts) == 2 && parts[0] == "usage_usec" {
		if stats.CPUUsageUsec, err = strconv.ParseInt(parts[1], 10, 64); err != nil {
			return nil, fmt.Errorf("parsing usage_usec %q: %w", lines[4], err)
		}
	}
	if parts := strings.Fields(lines[5]); len(parts) == 2 && parts[0] == "nr_throttled" {
		if stats.CPUThrottledCount, err = strconv.ParseInt(parts[1], 10, 64); err != nil {
			return nil, fmt.Errorf("parsing nr_throttled %q: %w", lines[5], err)
		}
	}
	if stats.PIDCount, err = strconv.ParseInt(strings.TrimSpace(lines[6]), 10, 64); err != nil {
		return nil, fmt.Errorf("parsing pids.current %q: %w", lines[6], err)
	}
	return stats, nil
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
