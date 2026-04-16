package subagents

import "strings"

// MapProviderToGroup maps a credential's provider ID (and optionally its model)
// to a prompt group. For direct providers the model is ignored. For aggregators
// (openrouter, fireworks-ai, groq) the model name is parsed to determine which
// provider family the model belongs to.
func MapProviderToGroup(providerID, model string) string {
	switch providerID {
	// Direct providers
	case "anthropic":
		return "anthropic"
	case "openai":
		return "openai"
	case "google", "google-vertex":
		return "gemini"
	case "moonshotai", "kimi":
		return "kimi"
	case "minimax":
		return "minimax"
	case "zai", "zhipuai", "glm":
		return "glm"

	// Aggregators — resolve from model name
	case "openrouter", "groq":
		return groupFromModelPrefix(model)
	case "fireworks-ai":
		return groupFromFireworksModel(model)

	default:
		return "openai"
	}
}

// groupFromModelPrefix extracts the provider prefix from models like
// "anthropic/claude-opus-4.6" or "moonshotai/kimi-k2.5".
func groupFromModelPrefix(model string) string {
	idx := strings.Index(model, "/")
	if idx < 1 {
		return "openai"
	}
	prefix := model[:idx]
	switch prefix {
	case "anthropic":
		return "anthropic"
	case "openai":
		return "openai"
	case "google":
		return "gemini"
	case "moonshotai":
		return "kimi"
	case "z-ai":
		return "glm"
	case "minimax":
		return "minimax"
	default:
		return "openai"
	}
}

// groupFromFireworksModel resolves models like
// "accounts/fireworks/models/kimi-k2p5" by matching the model suffix.
func groupFromFireworksModel(model string) string {
	const prefix = "accounts/fireworks/models/"
	name := model
	if strings.HasPrefix(model, prefix) {
		name = model[len(prefix):]
	}
	switch {
	case strings.HasPrefix(name, "kimi"):
		return "kimi"
	case strings.HasPrefix(name, "glm"):
		return "glm"
	case strings.HasPrefix(name, "minimax"):
		return "minimax"
	default:
		return "openai"
	}
}
