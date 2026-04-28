package billing

import (
	"errors"
	"fmt"
	"math"
)

// ErrUnknownModel is returned when TokensToCredits is asked to price a model
// that isn't in the rate table. Callers must treat this as an operational
// error (model config missing) rather than billing the user zero.
var ErrUnknownModel = errors.New("billing: unknown model")

// ModelRates is the per-million-token pricing for a model in USD.
// Matches the format in business/pricing.md — easy to cross-reference.
type ModelRates struct {
	InputPerMTok  float64
	OutputPerMTok float64
}

// modelRates is the authoritative pricing table for platform-keys inference.
// Kept separate from internal/registry (which holds observability prices)
// so billing and display can diverge when needed — e.g. honoring a
// grandfathered rate for an existing subscription after the registry price
// changes.
//
// Add new models here as we enable them on platform credentials.
var modelRates = map[string]ModelRates{
	// Default model (2026-04). See business/pricing.md.
	"glm-5.1-precision": {InputPerMTok: 0.437, OutputPerMTok: 4.40},
}

// WebsitePagePriceCredits is the flat per-page charge for a website
// crawl. Set conservatively above worst-case observed Spider per-page
// COGS so we never under-bill on heavy pages. 2 credits = $0.002 user
// = $0.0005 COGS budget at 75% margin (vs ~$0.0003 observed worst).
const WebsitePagePriceCredits = 2

// TokensToCredits converts an LLM call's input/output token counts into the
// number of credits the org's ledger should be debited.
//
// Math matches business/pricing.md:
//
//	COGS    = in × $/Mtok_in/1e6  +  out × $/Mtok_out/1e6
//	credits = ceil(COGS × 4000)   // 1 credit = $0.001, 75% GM → cost budget $0.00025/credit
//
// Ceil is deliberate: we never underbill. Negative or zero token counts
// produce zero credits (idle calls can't cost anything).
func TokensToCredits(model string, inputTokens, outputTokens int64) (int64, error) {
	rates, ok := modelRates[model]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrUnknownModel, model)
	}
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	if inputTokens == 0 && outputTokens == 0 {
		return 0, nil
	}
	cost := float64(inputTokens)*rates.InputPerMTok/1_000_000 +
		float64(outputTokens)*rates.OutputPerMTok/1_000_000
	credits := math.Ceil(cost * 4_000)
	return int64(credits), nil
}

// IsKnownModel reports whether the given model has an entry in the rate
// table. Useful for admin-side validation before writing a system credential
// or agent config.
func IsKnownModel(model string) bool {
	_, ok := modelRates[model]
	return ok
}
