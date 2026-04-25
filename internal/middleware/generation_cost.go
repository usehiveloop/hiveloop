package middleware

import (
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/observe"
	"github.com/usehiveloop/hiveloop/internal/registry"
)

// cachedTokenDiscount is the per-provider multiplier applied to cached input
// tokens when computing observability cost. Providers advertise different
// cache-read discounts on their public price sheets.
var cachedTokenDiscount = map[string]float64{
	"anthropic":     0.10,
	"openai":        0.50,
	"google":        0.25,
	"google-vertex": 0.25,
}

// calculateCost computes USD cost for one LLM call using registry pricing.
// This is the observability figure written into model.Generation.Cost and
// shown in usage dashboards — NOT the source of truth for billing, which
// uses billing.TokensToCredits against its own rate table.
func calculateCost(reg *registry.Registry, providerID, modelID string, usage observe.UsageData) float64 {
	if reg == nil || providerID == "" || modelID == "" {
		return 0
	}
	provider, ok := reg.GetProvider(providerID)
	if !ok {
		return 0
	}
	m, ok := provider.Models[modelID]
	if !ok || m.Cost == nil {
		return 0
	}

	inputPrice := m.Cost.Input
	outputPrice := m.Cost.Output

	nonCachedInput := usage.InputTokens - usage.CachedTokens
	if nonCachedInput < 0 {
		nonCachedInput = 0
	}
	inputCost := float64(nonCachedInput) * inputPrice / 1_000_000

	discount := cachedTokenDiscount[providerID]
	if discount == 0 && usage.CachedTokens > 0 {
		discount = 1.0
	}
	cachedCost := float64(usage.CachedTokens) * inputPrice * discount / 1_000_000
	outputCost := float64(usage.OutputTokens) * outputPrice / 1_000_000

	return inputCost + cachedCost + outputCost
}

// extractAttribution reads token.meta to extract user_id and tags. Populated
// into the Generation row for observability filtering.
func extractAttribution(db *gorm.DB, jti string, gen *model.Generation) {
	var token model.Token
	if err := db.Select("meta").Where("jti = ?", jti).First(&token).Error; err != nil {
		return
	}
	if token.Meta == nil {
		return
	}
	if user, ok := token.Meta["user"].(string); ok {
		gen.UserID = user
	}
	if tags, ok := token.Meta["tags"].([]any); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				gen.Tags = append(gen.Tags, s)
			}
		}
	}
}
