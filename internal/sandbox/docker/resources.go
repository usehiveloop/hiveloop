package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"

	"github.com/usehivy/hivy/internal/sandbox"
)

const bytesPerGiB = int64(1024 * 1024 * 1024)

func resourceLimits(cpu, memoryGB int) container.Resources {
	resources := container.Resources{}
	if cpu > 0 {
		resources.NanoCPUs = int64(cpu) * 1_000_000_000
	}
	if memoryGB > 0 {
		resources.Memory = int64(memoryGB) * bytesPerGiB
	}
	return resources
}

func (d *Driver) GetResourceUsage(ctx context.Context, externalID string) (*sandbox.ResourceUsage, error) {
	stats, err := d.readStats(ctx, externalID)
	if err != nil {
		return nil, err
	}
	inspect, err := d.cli.ContainerInspect(ctx, externalID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, sandbox.ErrSandboxNotFound
		}
		return nil, fmt.Errorf("inspecting docker container %s: %w", externalID, err)
	}

	return &sandbox.ResourceUsage{
		MemoryLimitBytes:  int64(stats.MemoryStats.Limit),
		MemoryUsedBytes:   int64(stats.MemoryStats.Usage),
		MemoryPeakBytes:   int64(stats.MemoryStats.MaxUsage),
		CPUQuota:          cpuQuota(inspect.HostConfig),
		CPUUsageUsec:      int64(stats.CPUStats.CPUUsage.TotalUsage / 1000),
		CPUThrottledCount: int64(stats.CPUStats.ThrottlingData.ThrottledPeriods),
		PIDCount:          int64(stats.PidsStats.Current),
	}, nil
}

func (d *Driver) readStats(ctx context.Context, externalID string) (*container.StatsResponse, error) {
	body, err := d.cli.ContainerStats(ctx, externalID, false)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, sandbox.ErrSandboxNotFound
		}
		return nil, fmt.Errorf("reading docker stats %s: %w", externalID, err)
	}
	defer body.Body.Close()

	var stats container.StatsResponse
	if err := json.NewDecoder(body.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("decoding docker stats %s: %w", externalID, err)
	}
	return &stats, nil
}

func cpuQuota(hostConfig *container.HostConfig) string {
	if hostConfig == nil {
		return ""
	}
	if hostConfig.CPUQuota > 0 || hostConfig.CPUPeriod > 0 {
		return fmt.Sprintf("%d %d", hostConfig.CPUQuota, hostConfig.CPUPeriod)
	}
	if hostConfig.NanoCPUs > 0 {
		return strconv.FormatInt(hostConfig.NanoCPUs, 10)
	}
	return ""
}
