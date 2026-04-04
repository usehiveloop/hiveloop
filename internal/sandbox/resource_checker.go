package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/llmvault/llmvault/internal/model"
)

// cgroupCommand is the shell command executed inside each sandbox to collect
// cgroup v2 resource stats. Output is 7 lines in a fixed order:
//
//	1. memory.max        (bytes)
//	2. memory.current    (bytes)
//	3. memory.peak       (bytes)
//	4. cpu.max           (e.g. "100000 100000")
//	5. usage_usec <val>
//	6. nr_throttled <val>
//	7. pids.current
const cgroupCommand = `cat /sys/fs/cgroup/memory.max /sys/fs/cgroup/memory.current /sys/fs/cgroup/memory.peak && cat /sys/fs/cgroup/cpu.max && grep -E "usage_usec|nr_throttled" /sys/fs/cgroup/cpu.stat && cat /sys/fs/cgroup/pids.current`

// resourceStats holds parsed cgroup resource data from a sandbox.
type resourceStats struct {
	MemoryLimitBytes  int64
	MemoryUsedBytes   int64
	MemoryPeakBytes   int64
	CPUQuota          string
	CPUUsageUsec      int64
	CPUThrottledCount int64
	PIDCount          int64
}

// StartResourceChecker runs a background loop that periodically collects
// resource usage from all running sandboxes via cgroup v2 stats.
func (o *Orchestrator) StartResourceChecker(ctx context.Context) {
	interval := o.cfg.SandboxResourceCheckInterval
	if interval <= 0 {
		slog.Info("sandbox resource checker disabled (interval <= 0)")
		return
	}

	slog.Info("sandbox resource checker started", "interval", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("sandbox resource checker stopped")
			return
		case <-ticker.C:
			o.runResourceCheck(ctx)
		}
	}
}

// runResourceCheck queries all running sandboxes and collects their resource stats.
func (o *Orchestrator) runResourceCheck(ctx context.Context) {
	var sandboxes []model.Sandbox
	if err := o.db.Where("status = 'running'").Find(&sandboxes).Error; err != nil {
		slog.Error("resource check: failed to query sandboxes", "error", err)
		return
	}

	if len(sandboxes) == 0 {
		return
	}

	slog.Info("resource check: collecting stats", "sandbox_count", len(sandboxes))

	for i := range sandboxes {
		sb := &sandboxes[i]
		o.collectSandboxResources(ctx, sb)
	}
}

// collectSandboxResources executes the cgroup command inside a single sandbox,
// parses the output, and updates both the sandbox record and any associated pool record.
func (o *Orchestrator) collectSandboxResources(ctx context.Context, sb *model.Sandbox) {
	output, err := o.provider.ExecuteCommand(ctx, sb.ExternalID, cgroupCommand)
	if err != nil {
		slog.Warn("resource check: execute failed",
			"sandbox_id", sb.ID,
			"external_id", sb.ExternalID,
			"error", err,
		)
		return
	}

	stats, err := parseCgroupOutput(output)
	if err != nil {
		slog.Warn("resource check: parse failed",
			"sandbox_id", sb.ID,
			"output", output,
			"error", err,
		)
		return
	}

	now := time.Now()
	updates := map[string]any{
		"memory_limit_bytes":  stats.MemoryLimitBytes,
		"memory_used_bytes":   stats.MemoryUsedBytes,
		"memory_peak_bytes":   stats.MemoryPeakBytes,
		"cpu_quota":           stats.CPUQuota,
		"cpu_usage_usec":      stats.CPUUsageUsec,
		"cpu_throttled_count": stats.CPUThrottledCount,
		"pid_count":           stats.PIDCount,
		"resource_checked_at": now,
	}

	if err := o.db.Model(sb).Updates(updates).Error; err != nil {
		slog.Error("resource check: db update failed", "sandbox_id", sb.ID, "error", err)
		return
	}

	slog.Debug("resource check: collected",
		"sandbox_id", sb.ID,
		"mem_used_mb", stats.MemoryUsedBytes/(1024*1024),
		"mem_limit_mb", stats.MemoryLimitBytes/(1024*1024),
		"cpu_usage_usec", stats.CPUUsageUsec,
		"pids", stats.PIDCount,
	)
}

// parseCgroupOutput parses the 7-line output from cgroupCommand.
func parseCgroupOutput(output string) (*resourceStats, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 7 {
		return nil, fmt.Errorf("expected 7 lines, got %d", len(lines))
	}

	stats := &resourceStats{}
	var err error

	// Line 0: memory.max (can be "max" for unlimited)
	if lines[0] == "max" {
		stats.MemoryLimitBytes = 0
	} else {
		stats.MemoryLimitBytes, err = strconv.ParseInt(lines[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing memory.max %q: %w", lines[0], err)
		}
	}

	// Line 1: memory.current
	stats.MemoryUsedBytes, err = strconv.ParseInt(lines[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing memory.current %q: %w", lines[1], err)
	}

	// Line 2: memory.peak
	stats.MemoryPeakBytes, err = strconv.ParseInt(lines[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing memory.peak %q: %w", lines[2], err)
	}

	// Line 3: cpu.max (e.g. "100000 100000")
	stats.CPUQuota = lines[3]

	// Line 4: "usage_usec <value>"
	usageParts := strings.Fields(lines[4])
	if len(usageParts) == 2 && usageParts[0] == "usage_usec" {
		stats.CPUUsageUsec, err = strconv.ParseInt(usageParts[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing usage_usec %q: %w", lines[4], err)
		}
	}

	// Line 5: "nr_throttled <value>"
	throttledParts := strings.Fields(lines[5])
	if len(throttledParts) == 2 && throttledParts[0] == "nr_throttled" {
		stats.CPUThrottledCount, err = strconv.ParseInt(throttledParts[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing nr_throttled %q: %w", lines[5], err)
		}
	}

	// Line 6: pids.current
	stats.PIDCount, err = strconv.ParseInt(strings.TrimSpace(lines[6]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing pids.current %q: %w", lines[6], err)
	}

	return stats, nil
}
