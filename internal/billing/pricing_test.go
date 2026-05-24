package billing_test

import (
	"errors"
	"testing"

	"github.com/usehivy/hivy/internal/billing"
)

func TestCostUSDToCredits(t *testing.T) {
	for _, tc := range []struct {
		name string
		cost float64
		want int64
	}{
		{name: "zero", cost: 0, want: 0},
		{name: "negative", cost: -0.01, want: 0},
		{name: "sub credit", cost: 0.00084592, want: 1},
		{name: "exact credit", cost: 0.031, want: 31},
		{name: "ceil", cost: 0.03071, want: 31},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := billing.CostUSDToCredits(tc.cost); got != tc.want {
				t.Fatalf("CostUSDToCredits(%f) = %d, want %d", tc.cost, got, tc.want)
			}
		})
	}
}

func TestEstimateCostUSD_UsesRegistryRouteAndCachedTokens(t *testing.T) {
	cost, err := billing.EstimateCostUSD(nil, "openrouter", "deepseek-v4-flash", 5_740, 79, 5_248)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}
	if cost <= 0 {
		t.Fatalf("estimated cost = %f, want positive", cost)
	}
}

func TestEstimateCostUSD_UnknownModel(t *testing.T) {
	_, err := billing.EstimateCostUSD(nil, "openrouter", "claude-3-nonexistent", 1000, 100, 0)
	if !errors.Is(err, billing.ErrUnknownModel) {
		t.Fatalf("expected ErrUnknownModel, got %v", err)
	}
}

func TestEstimateCostUSD_ZeroTokensZeroCost(t *testing.T) {
	cost, err := billing.EstimateCostUSD(nil, "openrouter", "deepseek-v4-flash", 0, 0, 0)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}
	if cost != 0 {
		t.Fatalf("zero tokens cost = %f, want 0", cost)
	}
}

func TestIsKnownModel(t *testing.T) {
	if !billing.IsKnownModel("deepseek-v4-flash") {
		t.Error("IsKnownModel(deepseek-v4-flash) = false, want true")
	}
	if billing.IsKnownModel("claude-3-nonexistent") {
		t.Error("IsKnownModel(unknown) = true, want false")
	}
}
