package proxy

import (
	"math"
	"testing"

	"github.com/llmvault/llmvault/internal/registry"
)

func TestCalculateCost_OpenAI(t *testing.T) {
	reg := registry.Global()

	usage := UsageData{
		InputTokens:  1000,
		OutputTokens: 500,
	}

	cost := CalculateCost(reg, "openai", "gpt-4o", usage)
	if cost <= 0 {
		t.Errorf("expected positive cost for gpt-4o, got %f", cost)
	}

	// gpt-4o pricing from registry: verify cost is reasonable
	// input ~$2.50/M, output ~$10/M (approximate, varies)
	// 1000 input tokens + 500 output tokens should be small but non-zero
	if cost > 0.1 {
		t.Errorf("cost seems too high: %f", cost)
	}
}

func TestCalculateCost_WithCachedTokens_Anthropic(t *testing.T) {
	reg := registry.Global()

	// Without cache
	usageNoCached := UsageData{
		InputTokens:  1000,
		OutputTokens: 500,
	}
	costNoCached := CalculateCost(reg, "anthropic", "claude-sonnet-4-20250514", usageNoCached)

	// With cache (same total input but some cached)
	usageCached := UsageData{
		InputTokens:  1000,
		OutputTokens: 500,
		CachedTokens: 800,
	}
	costCached := CalculateCost(reg, "anthropic", "claude-sonnet-4-20250514", usageCached)

	// Cached should be cheaper (Anthropic gives 90% discount on cached tokens)
	if costCached >= costNoCached {
		t.Errorf("cached cost (%f) should be less than non-cached cost (%f)", costCached, costNoCached)
	}
}

func TestCalculateCost_WithCachedTokens_OpenAI(t *testing.T) {
	reg := registry.Global()

	usageNoCached := UsageData{
		InputTokens:  1000,
		OutputTokens: 500,
	}
	costNoCached := CalculateCost(reg, "openai", "gpt-4o", usageNoCached)

	usageCached := UsageData{
		InputTokens:  1000,
		OutputTokens: 500,
		CachedTokens: 800,
	}
	costCached := CalculateCost(reg, "openai", "gpt-4o", usageCached)

	if costCached >= costNoCached {
		t.Errorf("cached cost (%f) should be less than non-cached cost (%f)", costCached, costNoCached)
	}
}

func TestCalculateCost_ZeroTokens(t *testing.T) {
	reg := registry.Global()

	usage := UsageData{}
	cost := CalculateCost(reg, "openai", "gpt-4o", usage)
	if cost != 0 {
		t.Errorf("expected 0 cost for zero tokens, got %f", cost)
	}
}

func TestCalculateCost_UnknownModel(t *testing.T) {
	reg := registry.Global()

	usage := UsageData{InputTokens: 1000, OutputTokens: 500}
	cost := CalculateCost(reg, "openai", "gpt-nonexistent-model", usage)
	if cost != 0 {
		t.Errorf("expected 0 cost for unknown model, got %f", cost)
	}
}

func TestCalculateCost_UnknownProvider(t *testing.T) {
	reg := registry.Global()

	usage := UsageData{InputTokens: 1000, OutputTokens: 500}
	cost := CalculateCost(reg, "nonexistent-provider", "some-model", usage)
	if cost != 0 {
		t.Errorf("expected 0 cost for unknown provider, got %f", cost)
	}
}

func TestCalculateCost_EmptyProviderID(t *testing.T) {
	reg := registry.Global()

	usage := UsageData{InputTokens: 1000, OutputTokens: 500}
	cost := CalculateCost(reg, "", "gpt-4o", usage)
	if cost != 0 {
		t.Errorf("expected 0 cost for empty provider, got %f", cost)
	}
}

func TestCalculateCost_EmptyModelID(t *testing.T) {
	reg := registry.Global()

	usage := UsageData{InputTokens: 1000, OutputTokens: 500}
	cost := CalculateCost(reg, "openai", "", usage)
	if cost != 0 {
		t.Errorf("expected 0 cost for empty model, got %f", cost)
	}
}

func TestCalculateCost_NilRegistry(t *testing.T) {
	usage := UsageData{InputTokens: 1000, OutputTokens: 500}
	cost := CalculateCost(nil, "openai", "gpt-4o", usage)
	if cost != 0 {
		t.Errorf("expected 0 cost for nil registry, got %f", cost)
	}
}

func TestCalculateCost_KnownPricing(t *testing.T) {
	// Build a test registry with known pricing
	reg := registry.Global()

	// Find a model that we know has pricing
	provider, ok := reg.GetProvider("openai")
	if !ok {
		t.Skip("openai provider not found in registry")
	}

	// Find a model with known cost
	for modelID, model := range provider.Models {
		if model.Cost == nil || model.Cost.Input == 0 {
			continue
		}

		usage := UsageData{
			InputTokens:  1_000_000, // exactly 1M tokens
			OutputTokens: 1_000_000,
		}

		cost := CalculateCost(reg, "openai", modelID, usage)

		// For exactly 1M tokens, cost should equal input_price + output_price
		expected := model.Cost.Input + model.Cost.Output
		if math.Abs(cost-expected) > 0.0001 {
			t.Errorf("model %s: cost = %f, expected %f (input=%f, output=%f)",
				modelID, cost, expected, model.Cost.Input, model.Cost.Output)
		}
		return // test one model is sufficient
	}

	t.Skip("no openai model with known pricing found")
}

func TestCalculateCost_CachedMoreThanInput(t *testing.T) {
	// Edge case: cached_tokens > input_tokens (shouldn't happen, but be safe)
	reg := registry.Global()

	usage := UsageData{
		InputTokens:  100,
		OutputTokens: 50,
		CachedTokens: 200, // more than input
	}

	cost := CalculateCost(reg, "openai", "gpt-4o", usage)
	// Should not be negative
	if cost < 0 {
		t.Errorf("cost should not be negative, got %f", cost)
	}
}
