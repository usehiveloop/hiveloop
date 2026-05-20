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
		Name:       "employee-test",
		SnapshotID: "employee-snapshot",
		EnvVars: map[string]string{
			"RUNTIME_SECRET":     "secret",
			"HIVY_PROXY_API_KEY": "ptok_test",
			"AGENT_API_KEY_ENV":  "HIVY_PROXY_API_KEY",
		},
		Labels: map[string]string{"harness": "employee-sandbox"},
	}

	params := snapshotParamsFromCreateOpts(opts)
	if params.Snapshot != opts.SnapshotID {
		t.Fatalf("snapshot = %q, want %q", params.Snapshot, opts.SnapshotID)
	}
	if !reflect.DeepEqual(params.EnvVars, opts.EnvVars) {
		t.Fatalf("env vars = %#v, want %#v", params.EnvVars, opts.EnvVars)
	}
	opts.EnvVars["RUNTIME_SECRET"] = "mutated"
	if params.EnvVars["RUNTIME_SECRET"] != "secret" {
		t.Fatalf("params env should be copied, got %q", params.EnvVars["RUNTIME_SECRET"])
	}
}
