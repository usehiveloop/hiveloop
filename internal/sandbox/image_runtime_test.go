package sandbox

import (
	"context"
	"strings"
	"testing"

	"github.com/caarlos0/env/v11"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestImageRuntimeContract(t *testing.T) {
	orch, provider, db := setupOrchestrator(t)
	org := createTestOrg(t, db)
	cred := createTestCred(t, db, org.ID)
	agent := createTestAgent(t, db, org.ID, cred.ID)

	ctx := context.Background()
	sb, err := orch.CreateCloudAgentSandbox(ctx, &agent)
	if err != nil {
		t.Fatalf("CreateCloudAgentSandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
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
		"BRIDGE_STORAGE_PATH": "/work/bridge.db",
		"HIVY_GIT_USERNAME":   sanitizeName(agent.Name),
	}
	for k, want := range required {
		if got[k] != want {
			t.Errorf("env %s = %q, want %q", k, got[k], want)
		}
	}
	if want := sanitizeName(agent.Name) + "@users.noreply.github.com"; got["HIVY_GIT_EMAIL"] != want {
		t.Errorf("env HIVY_GIT_EMAIL = %q, want %q", got["HIVY_GIT_EMAIL"], want)
	}

	dropped := []string{
		"BRIDGE_LSP_PORT",
		"BRIDGE_LSP_ADDR",
		"HIVY_LSP_PORT",
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

	want := "hivy-bridge-1-0-0-small-v1"
	if orch.cfg.CloudAgentsSandboxBaseImagePrefix != want {
		t.Errorf("cfg.CloudAgentsSandboxBaseImagePrefix = %q, want %q",
			orch.cfg.CloudAgentsSandboxBaseImagePrefix, want)
	}

	got2 := orch.resolveTemplateRef(&agent)
	if got2 != want {
		t.Errorf("resolveTemplateRef(agent without template) = %q, want %q", got2, want)
	}
}

// TestConfigImagePrefixDefault locks the env-default for the cloud agent
// image prefix. caarlos0/env keeps populating defaults even when required
// fields error out, so the err itself is fine to ignore here.
func TestConfigImagePrefixDefault(t *testing.T) {
	var cfg config.Config
	if err := env.Parse(&cfg); err != nil {
		t.Logf("env.Parse warned: %v (expected — required vars missing)", err)
	}

	want := "hivy-bridge-1-0-0-small-v1"
	if cfg.CloudAgentsSandboxBaseImagePrefix != want {
		t.Errorf("CloudAgentsSandboxBaseImagePrefix default = %q, want %q",
			cfg.CloudAgentsSandboxBaseImagePrefix, want)
	}
}
