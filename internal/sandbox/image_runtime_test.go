package sandbox

import (
	"context"
	"strings"
	"testing"

	"github.com/caarlos0/env/v11"

	"github.com/usehiveloop/hiveloop/internal/config"
	"github.com/usehiveloop/hiveloop/internal/model"
)

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

	if BridgePort != 25434 {
		t.Errorf("BridgePort = %d, want 25434", BridgePort)
	}

	if len(provider.createCalls) != 1 {
		t.Fatalf("provider.createCalls: got %d, want 1", len(provider.createCalls))
	}
	got := provider.createCalls[0].EnvVars

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

	dropped := []string{
		"BRIDGE_LSP_PORT",
		"BRIDGE_LSP_ADDR",
		"HIVELOOP_LSP_PORT",
		"BRIDGE_STORAGE_PATH",
	}
	for _, k := range dropped {
		if v, ok := got[k]; ok {
			t.Errorf("env %s should have been dropped, but got %q", k, v)
		}
	}

	if !strings.HasSuffix(got["BRIDGE_LISTEN_ADDR"], ":25434") {
		t.Errorf("BRIDGE_LISTEN_ADDR %q must end with :25434", got["BRIDGE_LISTEN_ADDR"])
	}

	if len(provider.endpointPorts) == 0 {
		t.Fatal("provider.GetEndpoint was never called")
	}
	for i, p := range provider.endpointPorts {
		if p != 25434 {
			t.Errorf("GetEndpoint call %d used port %d, want 25434", i, p)
		}
	}

	want := "hiveloop-bridge-1-0-0-small-v1"
	if orch.cfg.BridgeBaseDedicatedImagePrefix != want {
		t.Errorf("cfg.BridgeBaseDedicatedImagePrefix = %q, want %q",
			orch.cfg.BridgeBaseDedicatedImagePrefix, want)
	}

	got2 := orch.resolveSnapshot(&agent)
	if got2 != want {
		t.Errorf("resolveSnapshot(agent without template) = %q, want %q", got2, want)
	}
}

// TestConfigImagePrefixDefault locks the env-default for the dedicated
// image prefix. caarlos0/env keeps populating defaults even when required
// fields error out, so the err itself is fine to ignore here.
func TestConfigImagePrefixDefault(t *testing.T) {
	var cfg config.Config
	if err := env.Parse(&cfg); err != nil {
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
