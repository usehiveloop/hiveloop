package sandbox

import (
	bridgepkg "github.com/usehiveloop/hiveloop/internal/bridge"
	"github.com/usehiveloop/hiveloop/internal/registry"
)

var providerCheckpointModel = map[string]string{
	"anthropic":    "claude-3-5-haiku-latest",
	"google":       "gemini-2.5-flash",
	"openrouter":   "google/gemini-2.5-flash",
	"openai":       "gpt-5.1-codex-mini",
	"groq":         "openai/gpt-oss-20b",
	"fireworks":    "accounts/fireworks/models/minimax-m2p1",
	"fireworks-ai": "accounts/fireworks/models/minimax-m2p1",
	"moonshotai":   "kimi-k2-thinking",
	"zai":          "glm-4.7-flash",
	"zhipuai":      "glm-4.7-flash",
}

const (
	immortalTokenBudgetFraction = 0.50
	fallbackParentContextWindow = 128_000
)

func applyImmortalDefault(
	cfg *bridgepkg.AgentConfig,
	primary bridgepkg.ProviderConfig,
	providerID, primaryModel string,
) {
	if cfg == nil || cfg.Immortal != nil {
		return
	}
	checkpointModel, ok := providerCheckpointModel[providerID]
	if !ok {
		return
	}

	tokenBudget := int32(float64(parentContextWindow(providerID, primaryModel)) * immortalTokenBudgetFraction)

	cfg.Immortal = &bridgepkg.ImmortalConfig{
		CheckpointProvider: bridgepkg.ProviderConfig{
			ProviderType: primary.ProviderType,
			Model:        checkpointModel,
			ApiKey:       primary.ApiKey,
			BaseUrl:      primary.BaseUrl,
		},
		TokenBudget: &tokenBudget,
	}
}

func parentContextWindow(providerID, modelID string) int64 {
	if prov, ok := registry.Global().GetProvider(providerID); ok {
		if m, ok := prov.Models[modelID]; ok && m.Limit != nil && m.Limit.Context > 0 {
			return m.Limit.Context
		}
	}
	return fallbackParentContextWindow
}
