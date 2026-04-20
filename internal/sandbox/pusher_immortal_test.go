package sandbox

import (
	"testing"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
)

// applyImmortalDefault is the single place where every pushed agent gets its
// ImmortalConfig auto-generated. A miss here means production agents would
// never chain — they'd either compact or run out of context — so the map
// and the token-budget math are both load-bearing.

func TestApplyImmortalDefault_SetsCheckpointModelPerProvider(t *testing.T) {
	cases := []struct {
		providerID          string
		primaryModel        string
		wantCheckpointModel string
	}{
		{"anthropic", "claude-sonnet-4-6", "claude-3-5-haiku-latest"},
		{"google", "gemini-3-pro-preview", "gemini-2.5-flash"},
		{"openrouter", "anthropic/claude-sonnet-4.6", "google/gemini-2.5-flash"},
		{"openai", "gpt-5.1", "gpt-5.1-codex-mini"},
		{"groq", "moonshotai/kimi-k2-instruct-0905", "openai/gpt-oss-20b"},
		{"fireworks-ai", "accounts/fireworks/models/kimi-k2p5", "accounts/fireworks/models/minimax-m2p1"},
		{"moonshotai", "kimi-k2.5", "kimi-k2-thinking"},
		{"zai", "glm-5", "glm-4.7-flash"},
		{"zhipuai", "glm-5", "glm-4.7-flash"},
	}
	for _, tc := range cases {
		cfg := &bridgepkg.AgentConfig{}
		primary := bridgepkg.ProviderConfig{
			ProviderType: bridgepkg.Google,
			Model:        tc.primaryModel,
			ApiKey:       "ptok_abc",
		}
		applyImmortalDefault(cfg, primary, tc.providerID, tc.primaryModel)
		if cfg.Immortal == nil {
			t.Fatalf("%s: expected Immortal to be populated", tc.providerID)
		}
		if cfg.Immortal.CheckpointProvider.Model != tc.wantCheckpointModel {
			t.Errorf("%s: checkpoint model = %q, want %q",
				tc.providerID, cfg.Immortal.CheckpointProvider.Model, tc.wantCheckpointModel)
		}
	}
}

func TestApplyImmortalDefault_MirrorsPrimaryProviderTransport(t *testing.T) {
	// CheckpointProvider must share the primary's transport (ProviderType,
	// ApiKey, BaseUrl) so our proxy can route checkpoint calls through the
	// same credential-backed tunnel.
	baseURL := "https://proxy.example.com"
	primary := bridgepkg.ProviderConfig{
		ProviderType: bridgepkg.Google,
		Model:        "gemini-3-pro-preview",
		ApiKey:       "ptok_abc",
		BaseUrl:      &baseURL,
	}
	cfg := &bridgepkg.AgentConfig{}
	applyImmortalDefault(cfg, primary, "google", "gemini-3-pro-preview")

	cp := cfg.Immortal.CheckpointProvider
	if cp.ProviderType != primary.ProviderType {
		t.Errorf("ProviderType: got %v want %v", cp.ProviderType, primary.ProviderType)
	}
	if cp.ApiKey != primary.ApiKey {
		t.Errorf("ApiKey mismatch: got %q want %q", cp.ApiKey, primary.ApiKey)
	}
	if cp.BaseUrl == nil || *cp.BaseUrl != baseURL {
		t.Errorf("BaseUrl mismatch: got %v want %q", cp.BaseUrl, baseURL)
	}
}

func TestApplyImmortalDefault_TokenBudgetIs50PctOfParentContextWindow(t *testing.T) {
	// The chain-reset threshold was tuned down from 70% to 50% after observing
	// that history growth is the dominant cost driver on long conversations;
	// resetting earlier (and paying the checkpoint-extraction cost sooner)
	// is cheaper than running many turns against a 140k-token history.
	cases := []struct {
		name       string
		providerID string
		model      string
		wantBudget int32 // 50% of the registry's context window
	}{
		// gemini-3-pro-preview has context=1_000_000 in the registry.
		{"gemini-3-pro", "google", "gemini-3-pro-preview", 500000},
		// claude-sonnet-4-5 has context=200_000.
		{"claude-sonnet-4-5", "anthropic", "claude-sonnet-4-5", 100000},
		// gpt-5.1 has context=400_000.
		{"gpt-5.1", "openai", "gpt-5.1", 200000},
		// Unknown model hits the 128k fallback → 64,000.
		{"unknown-model", "openai", "something-we-havent-curated", 64000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &bridgepkg.AgentConfig{}
			primary := bridgepkg.ProviderConfig{Model: tc.model}
			applyImmortalDefault(cfg, primary, tc.providerID, tc.model)
			if cfg.Immortal == nil || cfg.Immortal.TokenBudget == nil {
				t.Fatalf("TokenBudget not set")
			}
			if *cfg.Immortal.TokenBudget != tc.wantBudget {
				t.Errorf("TokenBudget: got %d want %d", *cfg.Immortal.TokenBudget, tc.wantBudget)
			}
		})
	}
}

func TestApplyImmortalDefault_RespectsAuthorOverride(t *testing.T) {
	// If the author already set Immortal on the agent config, we must not
	// touch it — auto-generation is a default, not a replacement.
	customBudget := int32(50_000)
	existing := &bridgepkg.ImmortalConfig{TokenBudget: &customBudget}
	cfg := &bridgepkg.AgentConfig{Immortal: existing}
	applyImmortalDefault(cfg, bridgepkg.ProviderConfig{}, "google", "gemini-3-pro-preview")
	if cfg.Immortal != existing {
		t.Errorf("author-supplied Immortal was overwritten")
	}
	if *cfg.Immortal.TokenBudget != 50_000 {
		t.Errorf("TokenBudget changed from author's value")
	}
}

func TestApplyImmortalDefault_SkipsUnsupportedProviders(t *testing.T) {
	// A provider without an entry in providerCheckpointModel — e.g. ollama
	// (local inference) or an unknown custom provider — gets no auto
	// config. Authors can still set it manually if they want.
	cfg := &bridgepkg.AgentConfig{}
	applyImmortalDefault(cfg, bridgepkg.ProviderConfig{}, "ollama", "llama3")
	if cfg.Immortal != nil {
		t.Errorf("unsupported provider should not get auto-immortal config")
	}
	cfg2 := &bridgepkg.AgentConfig{}
	applyImmortalDefault(cfg2, bridgepkg.ProviderConfig{}, "some-new-provider", "some-model")
	if cfg2.Immortal != nil {
		t.Errorf("unknown provider should not get auto-immortal config")
	}
}

func TestApplyImmortalDefault_NilConfigIsNoOp(t *testing.T) {
	// Defensive: a caller that forgot to initialize AgentConfig shouldn't
	// panic us — we silently skip.
	applyImmortalDefault(nil, bridgepkg.ProviderConfig{}, "google", "gemini-3-pro-preview")
}
