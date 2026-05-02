package sandbox

import (
	"context"
	"strings"
	"testing"

	"github.com/caarlos0/env/v11"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
)

// TestImageRuntimeContract is a real integration test for the Wave-1
// useportal.bridge@rip-harness migration. It exercises the full
// CreateDedicatedSandbox path against a Postgres-backed model and the
// in-memory mockProvider, and asserts on the exact runtime contract that
// the new ACP-harness image is rebuilt for:
//
//  1. BridgePort = 25434 (preserved across the migration).
//  2. baseEnvVars passed to the provider include the new ACP-harness vars
//     (HOME, CLAUDE_CONFIG_DIR, OPENCODE_CONFIG_DIR, NO_BROWSER) and have
//     dropped any old LSP-related vars.
//  3. The dedicated image prefix is the new "hiveloop-bridge-1-0-0-small-v1"
//     constant the orchestrator falls back to when the agent has no
//     custom SandboxTemplateID.
//  4. The endpoint is fetched at the new port (orchestrator → provider).
func TestImageRuntimeContract(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)

	ctx := context.Background()
	sb, err := orch.CreateDedicatedSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("CreateDedicatedSandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
		db.Where("org_id = ?", org.ID).Delete(&model.WorkspaceStorage{})
	})

	// 1. Port constant check.
	if BridgePort != 25434 {
		t.Errorf("BridgePort = %d, want 25434", BridgePort)
	}

	// 2. CreateSandbox should have been called exactly once with the new env-var set.
	if len(provider.createCalls) != 1 {
		t.Fatalf("provider.createCalls: got %d, want 1", len(provider.createCalls))
	}
	got := provider.createCalls[0].EnvVars

	// Must-have keys (present + non-empty for each).
	required := map[string]string{
		"HOME":                "/work",
		"CLAUDE_CONFIG_DIR":   "/work/.claude",
		"OPENCODE_CONFIG_DIR": "/work/.opencode",
		"NO_BROWSER":          "1",
		"BRIDGE_LISTEN_ADDR":  "0.0.0.0:25434",
	}
	for k, want := range required {
		if got[k] != want {
			t.Errorf("env %s = %q, want %q", k, got[k], want)
		}
	}

	// Must-not-have keys (these were dropped: LSP-flavored env, old storage path).
	dropped := []string{
		"BRIDGE_LSP_PORT",
		"BRIDGE_LSP_ADDR",
		"HIVELOOP_LSP_PORT",
		"BRIDGE_STORAGE_PATH", // /home/daytona/.bridge/storage is gone, bridge uses workdir SQLite now
	}
	for _, k := range dropped {
		if v, ok := got[k]; ok {
			t.Errorf("env %s should have been dropped, but got %q", k, v)
		}
	}

	// Sanity: the listen-addr port matches the BridgePort const.
	if !strings.HasSuffix(got["BRIDGE_LISTEN_ADDR"], ":25434") {
		t.Errorf("BRIDGE_LISTEN_ADDR %q must end with :25434", got["BRIDGE_LISTEN_ADDR"])
	}

	// 3. Endpoint resolution must have been called with port 25434.
	if len(provider.endpointPorts) == 0 {
		t.Fatal("provider.GetEndpoint was never called")
	}
	for i, p := range provider.endpointPorts {
		if p != 25434 {
			t.Errorf("GetEndpoint call %d used port %d, want 25434", i, p)
		}
	}

	// 4. Image prefix the orchestrator falls back to should match the new constant.
	want := "hiveloop-bridge-1-0-0-small-v1"
	if orch.cfg.BridgeBaseDedicatedImagePrefix != want {
		t.Errorf("cfg.BridgeBaseDedicatedImagePrefix = %q, want %q",
			orch.cfg.BridgeBaseDedicatedImagePrefix, want)
	}

	// And the resolved snapshot for an agent with no custom template should be that prefix.
	got2 := orch.resolveSnapshot(&agent)
	if got2 != want {
		t.Errorf("resolveSnapshot(agent without template) = %q, want %q", got2, want)
	}
}

// TestConfigImagePrefixDefault verifies the env-default for the dedicated
// image prefix is the new ACP-harness image — keeps the migration constant
// honest as a checked-in piece of the runtime contract.
//
// We can't call config.Load() because it requires many unrelated env vars,
// so we use the env library directly with envDefaults applied to a fresh
// struct value.
func TestConfigImagePrefixDefault(t *testing.T) {
	var cfg config.Config
	if err := env.Parse(&cfg); err != nil {
		// Required fields will fail; that's expected in this minimal env.
		// envDefaults are applied even when required-field parsing errors,
		// because caarlos0/env keeps populating defaults as it walks the
		// struct. So we only fail this test if the *default* itself is wrong.
		t.Logf("env.Parse warned: %v (expected — required vars missing)", err)
	}

	want := "hiveloop-bridge-1-0-0-small-v1"
	if cfg.BridgeBaseDedicatedImagePrefix != want {
		t.Errorf("BridgeBaseDedicatedImagePrefix default = %q, want %q",
			cfg.BridgeBaseDedicatedImagePrefix, want)
	}
	if cfg.BridgeBaseImagePrefix != want {
		t.Errorf("BridgeBaseImagePrefix default = %q, want %q",
			cfg.BridgeBaseImagePrefix, want)
	}
}
