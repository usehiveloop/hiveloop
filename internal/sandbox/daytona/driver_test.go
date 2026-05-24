package daytona

import (
	"reflect"
	"testing"

	"github.com/usehivy/hivy/internal/sandbox"
)

// TestDriverImplementsProvider verifies at compile time that Driver satisfies sandbox.Provider.
func TestDriverImplementsProvider(t *testing.T) {
	var _ sandbox.Provider = (*Driver)(nil)
}

func TestSnapshotParamsFromCreateOpts_PassesEnvVarsUnchanged(t *testing.T) {
	opts := sandbox.CreateSandboxOpts{
		Name:        "employee-test",
		TemplateRef: "employee-snapshot",
		EnvVars: map[string]string{
			"RUNTIME_SECRET":     "secret",
			"HIVY_PROXY_API_KEY": "ptok_test",
			"AGENT_API_KEY_ENV":  "HIVY_PROXY_API_KEY",
		},
		Labels: map[string]string{"harness": "employee-sandbox"},
	}

	params := snapshotParamsFromCreateOpts(opts)
	if params.Snapshot != opts.TemplateRef {
		t.Fatalf("snapshot = %q, want %q", params.Snapshot, opts.TemplateRef)
	}
	if !reflect.DeepEqual(params.EnvVars, opts.EnvVars) {
		t.Fatalf("env vars = %#v, want %#v", params.EnvVars, opts.EnvVars)
	}
	opts.EnvVars["RUNTIME_SECRET"] = "mutated"
	if params.EnvVars["RUNTIME_SECRET"] != "secret" {
		t.Fatalf("params env should be copied, got %q", params.EnvVars["RUNTIME_SECRET"])
	}
}

func TestParseCgroupResourceUsage(t *testing.T) {
	output := "2147483648\n24600576\n24739840\n100000 100000\nusage_usec 1607312\nnr_throttled 0\n18\n"

	stats, err := parseCgroupResourceUsage(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.MemoryLimitBytes != 2147483648 {
		t.Errorf("MemoryLimitBytes: got %d", stats.MemoryLimitBytes)
	}
	if stats.MemoryUsedBytes != 24600576 {
		t.Errorf("MemoryUsedBytes: got %d", stats.MemoryUsedBytes)
	}
	if stats.CPUQuota != "100000 100000" {
		t.Errorf("CPUQuota: got %q", stats.CPUQuota)
	}
	if stats.CPUUsageUsec != 1607312 {
		t.Errorf("CPUUsageUsec: got %d", stats.CPUUsageUsec)
	}
	if stats.PIDCount != 18 {
		t.Errorf("PIDCount: got %d", stats.PIDCount)
	}
}
