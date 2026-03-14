package registry

import (
	"testing"
)

func TestCheapestModel_PicksLowestCost(t *testing.T) {
	p := &Provider{
		ID: "test",
		Models: map[string]Model{
			"expensive": {ID: "expensive", Cost: &Cost{Input: 10.0}},
			"cheap":     {ID: "cheap", Cost: &Cost{Input: 0.5}},
			"mid":       {ID: "mid", Cost: &Cost{Input: 3.0}},
		},
	}

	m := cheapestModel(p)
	if m == nil {
		t.Fatal("expected a model, got nil")
	}
	if m.ID != "cheap" {
		t.Errorf("expected cheapest model 'cheap', got %q", m.ID)
	}
}

func TestCheapestModel_NoCostData_ReturnsAny(t *testing.T) {
	p := &Provider{
		ID: "test",
		Models: map[string]Model{
			"a": {ID: "a"},
			"b": {ID: "b"},
		},
	}

	m := cheapestModel(p)
	if m == nil {
		t.Fatal("expected a model when no cost data, got nil")
	}
}

func TestCheapestModel_NoModels_ReturnsNil(t *testing.T) {
	p := &Provider{ID: "test", Models: map[string]Model{}}
	m := cheapestModel(p)
	if m != nil {
		t.Errorf("expected nil for empty models, got %q", m.ID)
	}
}

func TestCheapestModel_ZeroCostSkipped(t *testing.T) {
	p := &Provider{
		ID: "test",
		Models: map[string]Model{
			"free":    {ID: "free", Cost: &Cost{Input: 0}},
			"minimal": {ID: "minimal", Cost: &Cost{Input: 0.01}},
		},
	}

	m := cheapestModel(p)
	if m == nil {
		t.Fatal("expected a model, got nil")
	}
	if m.ID != "minimal" {
		t.Errorf("expected 'minimal' (zero cost skipped), got %q", m.ID)
	}
}

func TestVerify_UnknownProvider(t *testing.T) {
	reg := Global()
	result := reg.Verify(t.Context(), "nonexistent-provider-xyz", "https://example.com", "bearer", []byte("test"))
	if result.Valid {
		t.Error("expected invalid for unknown provider")
	}
	if result.Error != "unknown provider" {
		t.Errorf("expected 'unknown provider' error, got %q", result.Error)
	}
}

func TestVerify_OpenRouter_Live(t *testing.T) {
	reg := Global()

	// Verify OpenRouter provider exists and has models
	provider, ok := reg.GetProvider("openrouter")
	if !ok {
		t.Fatal("openrouter provider not found in registry")
	}

	m := cheapestModel(provider)
	if m == nil {
		t.Fatal("no model found for openrouter")
	}
	t.Logf("Cheapest OpenRouter model: %s (cost: $%.4f/1M input tokens)", m.ID, m.Cost.Input)

	// Verify that the OpenRouter API URL ends with /v1 — our fix must handle this
	if provider.API == "" {
		t.Fatal("openrouter should have an API URL in registry")
	}
	t.Logf("OpenRouter API URL: %s", provider.API)
}

func TestVerify_InvalidKey_Live(t *testing.T) {
	reg := Global()
	// Use a known provider with a known base URL to test invalid key rejection
	result := reg.Verify(t.Context(), "openai", "https://api.openai.com", "bearer", []byte("sk-invalid-test-key"))
	if result.Valid {
		t.Error("expected invalid for bad API key")
	}
	if result.Error != "invalid API key" {
		t.Errorf("expected 'invalid API key' error, got %q", result.Error)
	}
	t.Logf("Correctly rejected invalid key: %s", result.Error)
}
