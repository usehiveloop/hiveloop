package registry

import "testing"

func TestResolveModel_ExplicitProviderRoutes(t *testing.T) {
	reg := Global()

	anthropic, ok := reg.ResolveModel("anthropic", "claude-sonnet-4.6")
	if !ok {
		t.Fatal("anthropic route not found")
	}
	if anthropic.UpstreamID != "claude-sonnet-4-6" {
		t.Fatalf("anthropic upstream = %q, want claude-sonnet-4-6", anthropic.UpstreamID)
	}
	if anthropic.Model.ID != "claude-sonnet-4.6" {
		t.Fatalf("canonical model id = %q", anthropic.Model.ID)
	}

	openrouter, ok := reg.ResolveModel("openrouter", "claude-sonnet-4.6")
	if !ok {
		t.Fatal("openrouter route not found")
	}
	if openrouter.UpstreamID != "anthropic/claude-sonnet-4.6" {
		t.Fatalf("openrouter upstream = %q, want anthropic/claude-sonnet-4.6", openrouter.UpstreamID)
	}
}

func TestResolveModel_SameProviderFallback(t *testing.T) {
	route, ok := Global().ResolveModel("openai", "gpt-5.4")
	if !ok {
		t.Fatal("openai fallback route not found")
	}
	if route.UpstreamID != "gpt-5.4" {
		t.Fatalf("upstream = %q, want gpt-5.4", route.UpstreamID)
	}
}

func TestCanonicalModelsForProviders_DeduplicatesExplicitRoutes(t *testing.T) {
	models := Global().CanonicalModelsForProviders([]string{"anthropic", "openrouter"})
	count := 0
	var providers []string
	upstreamCount := 0
	for _, model := range models {
		switch model.ID {
		case "claude-sonnet-4.6":
			count++
			providers = model.ProviderIDs
		case "anthropic/claude-sonnet-4.6":
			upstreamCount++
		}
	}
	if count != 1 {
		t.Fatalf("claude-sonnet-4.6 count = %d, want 1", count)
	}
	if upstreamCount != 0 {
		t.Fatalf("anthropic/claude-sonnet-4.6 upstream count = %d, want 0", upstreamCount)
	}
	if len(providers) != 2 || providers[0] != "anthropic" || providers[1] != "openrouter" {
		t.Fatalf("providers = %v, want [anthropic openrouter]", providers)
	}
}

func TestValidateCanonicalModelRejectsExplicitUpstreamAlias(t *testing.T) {
	err := Global().ValidateCanonicalModel("anthropic/claude-sonnet-4.6")
	if err == nil {
		t.Fatal("expected upstream alias to be rejected as a canonical model")
	}
}
