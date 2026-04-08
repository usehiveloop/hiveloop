package registry

import (
	"fmt"
	"testing"
)

func TestBestModelForForge_KnownProviders(t *testing.T) {
	reg := Global()

	// Verify the algorithm picks something for major providers.
	providers := []string{"anthropic", "openai", "google", "fireworks-ai", "togetherai", "deepseek", "openrouter"}
	for _, pid := range providers {
		model, ok := reg.BestModelForForge(pid)
		if !ok {
			t.Errorf("%s: expected a model, got none", pid)
			continue
		}
		t.Logf("%s → %s", pid, model)

		// Verify the picked model actually exists and has tool_call.
		provider, _ := reg.GetProvider(pid)
		m, exists := provider.Models[model]
		if !exists {
			t.Errorf("%s: picked model %q doesn't exist in provider", pid, model)
			continue
		}
		if !m.ToolCall {
			t.Errorf("%s: picked model %q doesn't have tool_call=true", pid, model)
		}
	}
}

func TestBestModelForForge_PrefersOpenWeight(t *testing.T) {
	reg := Global()

	// Inference providers should pick open weight models.
	inferenceProviders := []string{"fireworks-ai", "togetherai", "deepseek"}
	for _, pid := range inferenceProviders {
		model, ok := reg.BestModelForForge(pid)
		if !ok {
			t.Skipf("%s: no model found", pid)
			continue
		}
		provider, _ := reg.GetProvider(pid)
		m := provider.Models[model]
		if !m.OpenWeights {
			t.Errorf("%s: expected open weight model, got %q (open_weights=%v)", pid, model, m.OpenWeights)
		}
	}
}

func TestBestModelForForge_FrontierPicksMidTier(t *testing.T) {
	reg := Global()

	// Anthropic should NOT pick Opus (most expensive).
	model, ok := reg.BestModelForForge("anthropic")
	if !ok {
		t.Skip("anthropic: no model found")
	}
	provider, _ := reg.GetProvider("anthropic")
	m := provider.Models[model]
	if m.Cost != nil && m.Cost.Output > 20 {
		t.Errorf("anthropic: picked expensive model %q ($%.0f/M output), expected mid-tier", model, m.Cost.Output)
	}
	t.Logf("anthropic → %s ($%.0f/M output)", model, m.Cost.Output)
}

func TestBestModelForForge_UnknownProvider(t *testing.T) {
	reg := Global()
	_, ok := reg.BestModelForForge("nonexistent-provider-xyz")
	if ok {
		t.Error("expected no model for unknown provider")
	}
}

func TestBestModelForForge_Deterministic(t *testing.T) {
	reg := Global()

	// Run 10 times — same result each time.
	var first string
	for i := 0; i < 10; i++ {
		model, ok := reg.BestModelForForge("fireworks-ai")
		if !ok {
			t.Fatal("expected a model for fireworks-ai")
		}
		if i == 0 {
			first = model
		} else if model != first {
			t.Fatalf("non-deterministic: run %d got %q, expected %q", i, model, first)
		}
	}
	t.Logf("fireworks-ai → %s (consistent across 10 runs)", first)
}

func TestBestModelForForge_PrintAllPicks(t *testing.T) {
	reg := Global()

	providers := []string{
		"anthropic", "openai", "google", "fireworks-ai", "togetherai",
		"deepseek", "openrouter", "groq", "mistral", "cohere",
	}
	for _, pid := range providers {
		model, ok := reg.BestModelForForge(pid)
		if ok {
			provider, _ := reg.GetProvider(pid)
			m := provider.Models[model]
			costStr := "free"
			if m.Cost != nil {
				costStr = fmt.Sprintf("$%.2f/$%.2f", m.Cost.Input, m.Cost.Output)
			}
			t.Logf("%-15s → %-50s open=%-5v reason=%-5v %s", pid, model, m.OpenWeights, m.Reasoning, costStr)
		} else {
			t.Logf("%-15s → (none)", pid)
		}
	}
}
