package sandbox

import (
	"fmt"
	"strconv"
	"strings"
)

func parseCgroupOutput(output string) (*ResourceUsage, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 7 {
		return nil, fmt.Errorf("expected 7 lines, got %d", len(lines))
	}

	stats := &ResourceUsage{CPUQuota: lines[3]}
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
