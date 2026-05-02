package sandbox

import (
	"testing"

	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
)

// TestPusherHarnessSelection asserts the (provider, model) -> harness mapping
// is deterministic and matches the documented routing rules:
//   - Anthropic provider OR claude-prefixed model -> Claude harness.
//   - Everything else -> OpenCode harness.
//
// The model-prefix check wins over the provider type so an OpenAI-shaped
// proxy serving Claude (e.g. Bedrock-via-OpenAI) still lands on Claude.
func TestPusherHarnessSelection(t *testing.T) {
	cases := []struct {
		name     string
		provider bridgepkg.ProviderType
		model    string
		want     bridgepkg.Harness
	}{
		{"anthropic + claude-sonnet-4-6", bridgepkg.Anthropic, "claude-sonnet-4-6", bridgepkg.Claude},
		{"anthropic + claude-opus-4-7", bridgepkg.Anthropic, "claude-opus-4-7", bridgepkg.Claude},
		{"openai + gpt-4o", bridgepkg.OpenAi, "gpt-4o", bridgepkg.OpenCode},
		{"openai + claude-via-proxy: model prefix wins", bridgepkg.OpenAi, "claude-via-proxy", bridgepkg.Claude},
		{"google + gemini-1.5-pro", bridgepkg.Google, "gemini-1.5-pro", bridgepkg.OpenCode},
		{"groq + llama-3.1", bridgepkg.Groq, "llama-3.1", bridgepkg.OpenCode},
		{"empty model + anthropic provider", bridgepkg.Anthropic, "", bridgepkg.Claude},
		{"empty model + openai provider", bridgepkg.OpenAi, "", bridgepkg.OpenCode},
		{"custom provider + claude prefix", bridgepkg.Custom, "claude-haiku", bridgepkg.Claude},
		{"custom provider + non-claude", bridgepkg.Custom, "kimi-k2", bridgepkg.OpenCode},
		{"uppercase Claude prefix still matches", bridgepkg.OpenAi, "Claude-Sonnet", bridgepkg.Claude},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := harnessFor(tc.provider, tc.model)
			if got != tc.want {
				t.Errorf("harnessFor(%v, %q) = %q, want %q", tc.provider, tc.model, got, tc.want)
			}
		})
	}
}

// TestPusherResolveHarness asserts the persisted-harness path is preferred
// over the (provider, model) computation when the agent already has a
// non-empty Harness column.
func TestPusherResolveHarness_PrefersPersistedValue(t *testing.T) {
	// Even though provider+model would compute OpenCode, the persisted
	// value "claude" wins.
	got := resolveHarness("claude", bridgepkg.OpenAi, "gpt-4o")
	if got != bridgepkg.Claude {
		t.Errorf("resolveHarness with persisted=claude should return claude, got %q", got)
	}

	// Empty persisted value -> compute.
	got = resolveHarness("", bridgepkg.OpenAi, "gpt-4o")
	if got != bridgepkg.OpenCode {
		t.Errorf("resolveHarness with empty persisted should compute open_code, got %q", got)
	}

	got = resolveHarness("", bridgepkg.Anthropic, "claude-sonnet-4-6")
	if got != bridgepkg.Claude {
		t.Errorf("resolveHarness empty persisted + anthropic should compute claude, got %q", got)
	}
}
