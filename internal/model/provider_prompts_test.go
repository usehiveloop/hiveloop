package model

import (
	"testing"

	"github.com/google/uuid"
)

func TestResolveProviderConfig_EmptyProviderPrompts(t *testing.T) {
	agent := &Agent{
		SystemPrompt: "default prompt",
		Model:        "default-model",
	}

	prompt, model := agent.ResolveProviderConfig("anthropic")
	if prompt != "default prompt" {
		t.Errorf("expected default prompt, got %q", prompt)
	}
	if model != "default-model" {
		t.Errorf("expected default-model, got %q", model)
	}
}

func TestResolveProviderConfig_MatchingProvider(t *testing.T) {
	agent := &Agent{
		SystemPrompt: "default prompt",
		Model:        "default-model",
	}
	agent.SetProviderPrompts(map[string]ProviderPromptConfig{
		"anthropic": {SystemPrompt: "anthropic prompt", Model: "claude-sonnet-4"},
		"openai":    {SystemPrompt: "openai prompt", Model: "gpt-4o"},
	})

	prompt, model := agent.ResolveProviderConfig("anthropic")
	if prompt != "anthropic prompt" {
		t.Errorf("expected anthropic prompt, got %q", prompt)
	}
	if model != "claude-sonnet-4" {
		t.Errorf("expected claude-sonnet-4, got %q", model)
	}
}

func TestResolveProviderConfig_NonMatchingProvider(t *testing.T) {
	agent := &Agent{
		SystemPrompt: "default prompt",
		Model:        "default-model",
	}
	agent.SetProviderPrompts(map[string]ProviderPromptConfig{
		"anthropic": {SystemPrompt: "anthropic prompt", Model: "claude-sonnet-4"},
	})

	prompt, model := agent.ResolveProviderConfig("gemini")
	if prompt != "default prompt" {
		t.Errorf("expected fallback to default prompt, got %q", prompt)
	}
	if model != "default-model" {
		t.Errorf("expected fallback to default-model, got %q", model)
	}
}

func TestResolveProviderConfig_PartialConfig_OnlyPrompt(t *testing.T) {
	agent := &Agent{
		SystemPrompt: "default prompt",
		Model:        "default-model",
	}
	agent.SetProviderPrompts(map[string]ProviderPromptConfig{
		"kimi": {SystemPrompt: "kimi prompt", Model: ""},
	})

	prompt, model := agent.ResolveProviderConfig("kimi")
	if prompt != "kimi prompt" {
		t.Errorf("expected kimi prompt, got %q", prompt)
	}
	if model != "default-model" {
		t.Errorf("expected fallback to default-model when model is empty, got %q", model)
	}
}

func TestResolveProviderConfig_PartialConfig_OnlyModel(t *testing.T) {
	agent := &Agent{
		SystemPrompt: "default prompt",
		Model:        "default-model",
	}
	agent.SetProviderPrompts(map[string]ProviderPromptConfig{
		"minimax": {SystemPrompt: "", Model: "minimax-m1"},
	})

	prompt, model := agent.ResolveProviderConfig("minimax")
	if prompt != "default prompt" {
		t.Errorf("expected fallback to default prompt when system_prompt is empty, got %q", prompt)
	}
	if model != "minimax-m1" {
		t.Errorf("expected minimax-m1, got %q", model)
	}
}

func TestBridgeAgentID_SystemAgent(t *testing.T) {
	agentID := uuid.New()
	agent := &Agent{
		ID:       agentID,
		IsSystem: true,
	}

	bridgeID := agent.BridgeAgentID("anthropic")
	expected := agentID.String() + "-anthropic"
	if bridgeID != expected {
		t.Errorf("expected %q, got %q", expected, bridgeID)
	}
}

func TestBridgeAgentID_NonSystemAgent(t *testing.T) {
	agentID := uuid.New()
	agent := &Agent{
		ID:       agentID,
		IsSystem: false,
	}

	bridgeID := agent.BridgeAgentID("anthropic")
	if bridgeID != agentID.String() {
		t.Errorf("expected plain agent ID %q, got %q", agentID.String(), bridgeID)
	}
}

func TestBridgeAgentID_SystemAgent_EmptyProviderGroup(t *testing.T) {
	agentID := uuid.New()
	agent := &Agent{
		ID:       agentID,
		IsSystem: true,
	}

	bridgeID := agent.BridgeAgentID("")
	if bridgeID != agentID.String() {
		t.Errorf("expected plain agent ID when providerGroup is empty, got %q", bridgeID)
	}
}

func TestProviderPromptsMap_RoundTrip(t *testing.T) {
	agent := &Agent{}
	input := map[string]ProviderPromptConfig{
		"anthropic": {SystemPrompt: "anthropic prompt", Model: "claude-sonnet-4"},
		"openai":    {SystemPrompt: "openai prompt", Model: "gpt-4o"},
		"gemini":    {SystemPrompt: "gemini prompt", Model: "gemini-2.5-pro"},
	}

	if err := agent.SetProviderPrompts(input); err != nil {
		t.Fatalf("SetProviderPrompts: %v", err)
	}

	output := agent.ProviderPromptsMap()
	if len(output) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(output))
	}

	for provider, expectedConfig := range input {
		got, ok := output[provider]
		if !ok {
			t.Errorf("missing provider %q", provider)
			continue
		}
		if got.SystemPrompt != expectedConfig.SystemPrompt {
			t.Errorf("provider %q: expected prompt %q, got %q", provider, expectedConfig.SystemPrompt, got.SystemPrompt)
		}
		if got.Model != expectedConfig.Model {
			t.Errorf("provider %q: expected model %q, got %q", provider, expectedConfig.Model, got.Model)
		}
	}
}
