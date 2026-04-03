package forge

import (
	"embed"
	"fmt"
)

//go:embed prompts/*.md
var promptsFS embed.FS

// ForgeRole identifies which forge agent a prompt is for.
type ForgeRole string

const (
	RoleArchitect    ForgeRole = "architect"
	RoleEvalDesigner ForgeRole = "eval_designer"
	RoleJudge        ForgeRole = "judge"
)

// LoadSystemPrompt returns the system prompt for a forge agent role and target provider.
// For the judge, providerID is ignored (single universal prompt).
func LoadSystemPrompt(role ForgeRole, providerID string) (string, error) {
	var filename string
	if role == RoleJudge {
		filename = "prompts/judge.md"
	} else {
		group := mapProviderToGroup(providerID)
		filename = fmt.Sprintf("prompts/%s_%s.md", role, group)
	}

	data, err := promptsFS.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("loading prompt %s: %w", filename, err)
	}
	return string(data), nil
}

// mapProviderToGroup maps a credential's provider ID to one of the 6 forge
// prompt groups. Providers not in the 6 groups fall back to "openai" since
// they use OpenAI-compatible APIs.
func mapProviderToGroup(providerID string) string {
	switch providerID {
	case "anthropic":
		return "anthropic"
	case "openai":
		return "openai"
	case "google", "google-vertex":
		return "gemini"
	case "kimi":
		return "kimi"
	case "minimax":
		return "minimax"
	case "glm":
		return "glm"
	default:
		// deepseek, groq, fireworks, together, xai, mistral, cohere, ollama
		return "openai"
	}
}
