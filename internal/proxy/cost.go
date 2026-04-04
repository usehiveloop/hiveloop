package proxy

import (
	"github.com/ziraloop/ziraloop/internal/registry"
)

// Cached token discount factors by provider.
// These represent the fraction of the input price charged for cached tokens.
var cachedTokenDiscount = map[string]float64{
	"anthropic":    0.10, // 90% discount
	"openai":       0.50, // 50% discount
	"google":       0.25, // 75% discount
	"google-vertex": 0.25,
}

// CalculateCost computes the USD cost of a generation based on token usage
// and the provider's pricing from the registry.
func CalculateCost(reg *registry.Registry, providerID, modelID string, usage UsageData) float64 {
	if reg == nil || providerID == "" || modelID == "" {
		return 0
	}

	provider, ok := reg.GetProvider(providerID)
	if !ok {
		return 0
	}

	model, ok := provider.Models[modelID]
	if !ok {
		return 0
	}

	if model.Cost == nil {
		return 0
	}

	inputPrice := model.Cost.Input   // $/1M input tokens
	outputPrice := model.Cost.Output // $/1M output tokens

	// Calculate base input cost (non-cached tokens)
	nonCachedInput := usage.InputTokens - usage.CachedTokens
	if nonCachedInput < 0 {
		nonCachedInput = 0
	}
	inputCost := float64(nonCachedInput) * inputPrice / 1_000_000

	// Calculate cached token cost (discounted input rate)
	discount := cachedTokenDiscount[providerID]
	if discount == 0 && usage.CachedTokens > 0 {
		// Unknown provider with cached tokens — assume full input price
		discount = 1.0
	}
	cachedCost := float64(usage.CachedTokens) * inputPrice * discount / 1_000_000

	// Calculate output cost (reasoning tokens are part of output tokens, same price)
	outputCost := float64(usage.OutputTokens) * outputPrice / 1_000_000

	return inputCost + cachedCost + outputCost
}
