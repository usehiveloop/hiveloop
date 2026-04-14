package systemagents

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMapProviderToGroup(t *testing.T) {
	tests := []struct {
		name       string
		providerID string
		model      string
		expected   string
	}{
		// Direct providers
		{"anthropic", "anthropic", "", "anthropic"},
		{"openai", "openai", "", "openai"},
		{"google", "google", "", "gemini"},
		{"google-vertex", "google-vertex", "", "gemini"},
		{"kimi", "kimi", "", "kimi"},
		{"moonshotai", "moonshotai", "", "kimi"},
		{"minimax", "minimax", "", "minimax"},
		{"glm", "glm", "", "glm"},
		{"zai", "zai", "", "glm"},
		{"zhipuai", "zhipuai", "", "glm"},

		// OpenRouter — resolved from model prefix
		{"openrouter/anthropic", "openrouter", "anthropic/claude-opus-4.6", "anthropic"},
		{"openrouter/openai", "openrouter", "openai/gpt-5.4-pro", "openai"},
		{"openrouter/google", "openrouter", "google/gemini-3-pro-preview", "gemini"},
		{"openrouter/moonshotai", "openrouter", "moonshotai/kimi-k2.5", "kimi"},
		{"openrouter/z-ai", "openrouter", "z-ai/glm-5", "glm"},
		{"openrouter/minimax", "openrouter", "minimax/minimax-m2.5", "minimax"},
		{"openrouter/unknown", "openrouter", "deepseek/some-model", "openai"},
		{"openrouter/no-model", "openrouter", "", "openai"},

		// Groq — resolved from model prefix
		{"groq/moonshotai", "groq", "moonshotai/kimi-k2-instruct", "kimi"},
		{"groq/openai", "groq", "openai/gpt-oss-20b", "openai"},

		// Fireworks AI — resolved from model suffix
		{"fireworks/kimi", "fireworks-ai", "accounts/fireworks/models/kimi-k2p5", "kimi"},
		{"fireworks/glm", "fireworks-ai", "accounts/fireworks/models/glm-5", "glm"},
		{"fireworks/minimax", "fireworks-ai", "accounts/fireworks/models/minimax-m2p5", "minimax"},
		{"fireworks/unknown", "fireworks-ai", "accounts/fireworks/models/some-model", "openai"},

		// Fallback to openai for unknown providers
		{"deepseek", "deepseek", "", "openai"},
		{"mistral", "mistral", "", "openai"},
		{"together", "together", "", "openai"},
		{"xai", "xai", "", "openai"},
		{"cohere", "cohere", "", "openai"},
		{"ollama", "ollama", "", "openai"},
		{"unknown-provider", "unknown-provider", "", "openai"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapProviderToGroup(tt.providerID, tt.model)
			if got != tt.expected {
				t.Errorf("MapProviderToGroup(%q, %q) = %q, want %q", tt.providerID, tt.model, got, tt.expected)
			}
		})
	}
}

func TestYAMLDefinitions_AllParseable(t *testing.T) {
	// Verify all embedded YAML files can be read and parsed.
	entries, err := agentsFS.ReadDir(".")
	if err != nil {
		t.Fatalf("cannot read embedded FS root: %v", err)
	}

	var count int
	for _, typeDir := range entries {
		if !typeDir.IsDir() {
			continue
		}

		files, err := agentsFS.ReadDir(typeDir.Name())
		if err != nil {
			t.Fatalf("cannot read %s: %v", typeDir.Name(), err)
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}
			data, err := agentsFS.ReadFile(typeDir.Name() + "/" + file.Name())
			if err != nil {
				t.Errorf("cannot read %s/%s: %v", typeDir.Name(), file.Name(), err)
				continue
			}

			var af agentFile
			if err := yaml.Unmarshal(data, &af); err != nil {
				t.Errorf("cannot parse %s/%s: %v", typeDir.Name(), file.Name(), err)
				continue
			}
			if af.Model == "" {
				t.Errorf("%s/%s: missing model", typeDir.Name(), file.Name())
			}
			if af.SystemPrompt == "" {
				t.Errorf("%s/%s: missing system_prompt", typeDir.Name(), file.Name())
			}
			count++
		}
	}

	// 6 agent types x 6 providers + 1 zira (anthropic only) = 37 definitions.
	if count != 37 {
		t.Errorf("expected 37 YAML definitions, got %d", count)
	}
}
