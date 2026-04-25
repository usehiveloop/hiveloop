package billing_test

import (
	"errors"
	"testing"

	"github.com/usehiveloop/hiveloop/internal/billing"
)

func TestTokensToCredits_GLM5_1Precision(t *testing.T) {
	// Exact math from business/pricing.md "Typical" workload:
	//   30k input × $0.437/M + 4k output × $4.40/M = $0.01311 + $0.01760 = $0.03071
	//   + ~$0.004 sandbox (not priced here; see sandbox-minute billing)
	//   credits = ceil($0.03071 × 4000) = ceil(122.84) = 123
	//
	// Pricing.md cites ~139 credits for a typical run because it folds in a
	// $0.004 sandbox charge; this function only prices inference.
	credits, err := billing.TokensToCredits("glm-5.1-precision", 30_000, 4_000)
	if err != nil {
		t.Fatalf("TokensToCredits: %v", err)
	}
	if credits != 123 {
		t.Errorf("typical inference cost = %d credits, want 123", credits)
	}
}

func TestTokensToCredits_HeavyConversation(t *testing.T) {
	// 200k input + 15k output at GLM 5.1 rates:
	//   200k × 0.437/M + 15k × 4.40/M = $0.0874 + $0.066 = $0.1534
	//   credits = ceil($0.1534 × 4000) = ceil(613.6) = 614
	credits, err := billing.TokensToCredits("glm-5.1-precision", 200_000, 15_000)
	if err != nil {
		t.Fatalf("TokensToCredits: %v", err)
	}
	if credits != 614 {
		t.Errorf("heavy inference cost = %d credits, want 614", credits)
	}
}

func TestTokensToCredits_UnknownModel(t *testing.T) {
	_, err := billing.TokensToCredits("claude-3-nonexistent", 1000, 100)
	if !errors.Is(err, billing.ErrUnknownModel) {
		t.Fatalf("expected ErrUnknownModel, got %v", err)
	}
}

func TestTokensToCredits_ZeroTokensZeroCredits(t *testing.T) {
	credits, err := billing.TokensToCredits("glm-5.1-precision", 0, 0)
	if err != nil {
		t.Fatalf("TokensToCredits: %v", err)
	}
	if credits != 0 {
		t.Errorf("zero tokens = %d credits, want 0", credits)
	}
}

func TestTokensToCredits_NegativeTokensClampedToZero(t *testing.T) {
	credits, err := billing.TokensToCredits("glm-5.1-precision", -100, -50)
	if err != nil {
		t.Fatalf("TokensToCredits: %v", err)
	}
	if credits != 0 {
		t.Errorf("negative tokens should clamp to zero, got %d credits", credits)
	}
}

func TestTokensToCredits_AlwaysCeils(t *testing.T) {
	// 1 input token at $0.437/M = $0.000000437 → × 4000 = 0.001748 → ceil = 1.
	// Sub-credit amounts always round UP to 1, never down to 0.
	credits, err := billing.TokensToCredits("glm-5.1-precision", 1, 0)
	if err != nil {
		t.Fatalf("TokensToCredits: %v", err)
	}
	if credits != 1 {
		t.Errorf("1 input token = %d credits, want 1 (ceil)", credits)
	}
}

func TestIsKnownModel(t *testing.T) {
	if !billing.IsKnownModel("glm-5.1-precision") {
		t.Error("IsKnownModel(glm-5.1-precision) = false, want true")
	}
	if billing.IsKnownModel("claude-3-nonexistent") {
		t.Error("IsKnownModel(unknown) = true, want false")
	}
}
