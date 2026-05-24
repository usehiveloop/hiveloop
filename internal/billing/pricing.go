package billing

import (
	"errors"
	"fmt"
	"math"

	"github.com/usehivy/hivy/internal/registry"
)

const (
	// CreditUSDValue is the customer-facing value of one credit for AI usage.
	CreditUSDValue = 0.001

	CostSourceProvider = "provider_reported"
	CostSourceRegistry = "registry_estimated"

	// WebsitePagePriceCredits is the flat per-page charge for a website crawl.
	WebsitePagePriceCredits = 1
)

// ErrUnknownModel is returned when a generation has no provider-reported cost
// and the registry cannot estimate the model/provider route.
var ErrUnknownModel = errors.New("billing: unknown model")

var cachedTokenDiscount = map[string]float64{
	"anthropic":     0.10,
	"openai":        0.50,
	"google":        0.25,
	"google-vertex": 0.25,
}

func CostUSDToCredits(cost float64) int64 {
	if cost <= 0 {
		return 0
	}
	return int64(math.Ceil(cost / CreditUSDValue))
}

func EstimateCostUSD(reg *registry.Registry, providerID, modelID string, inputTokens, outputTokens, cachedTokens int64) (float64, error) {
	if reg == nil {
		reg = registry.Global()
	}
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	if cachedTokens < 0 {
		cachedTokens = 0
	}
	if inputTokens == 0 && outputTokens == 0 {
		return 0, nil
	}

	route, ok := reg.ResolveModel(providerID, modelID)
	if !ok || route.Model.Cost == nil {
		return 0, fmt.Errorf("%w: provider=%q model=%q", ErrUnknownModel, providerID, modelID)
	}

	if cachedTokens > inputTokens {
		cachedTokens = inputTokens
	}
	nonCachedInput := inputTokens - cachedTokens
	inputCost := float64(nonCachedInput) * route.Model.Cost.Input / 1_000_000
	discount := cachedTokenDiscount[providerID]
	if discount == 0 && cachedTokens > 0 {
		discount = 1
	}
	cachedCost := float64(cachedTokens) * route.Model.Cost.Input * discount / 1_000_000
	outputCost := float64(outputTokens) * route.Model.Cost.Output / 1_000_000
	return inputCost + cachedCost + outputCost, nil
}

func IsKnownModel(model string) bool {
	_, ok := registry.Global().CanonicalModel(model)
	return ok
}
