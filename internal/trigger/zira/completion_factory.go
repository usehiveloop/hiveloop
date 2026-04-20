package zira

import "github.com/usehiveloop/hiveloop/internal/model"

// NewCompletionClient returns the appropriate CompletionClient adapter for
// the given credential's provider. Anthropic gets its own adapter; all other
// providers use the OpenAI-compatible adapter (OpenAI, Fireworks, OpenRouter,
// DeepSeek, Groq, Together, xAI, Mistral, Cohere, etc.).
func NewCompletionClient(credential *model.Credential, decryptedKey string) CompletionClient {
	switch credential.ProviderID {
	case "anthropic":
		return NewAnthropicCompletionClient(decryptedKey)
	default:
		return NewOpenAICompletionClient(credential.BaseURL, decryptedKey)
	}
}
