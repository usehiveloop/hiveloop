package systemagents

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMapProviderToGroup(t *testing.T) {
	tests := []struct {
		providerID string
		expected   string
	}{
		{"anthropic", "anthropic"},
		{"openai", "openai"},
		{"google", "gemini"},
		{"google-vertex", "gemini"},
		{"kimi", "kimi"},
		{"minimax", "minimax"},
		{"glm", "glm"},
		// Fallback to openai for unknown/OpenAI-compatible providers.
		{"groq", "openai"},
		{"deepseek", "openai"},
		{"mistral", "openai"},
		{"fireworks", "openai"},
		{"together", "openai"},
		{"xai", "openai"},
		{"cohere", "openai"},
		{"ollama", "openai"},
		{"unknown-provider", "openai"},
	}

	for _, tt := range tests {
		t.Run(tt.providerID, func(t *testing.T) {
			got := MapProviderToGroup(tt.providerID)
			if got != tt.expected {
				t.Errorf("MapProviderToGroup(%q) = %q, want %q", tt.providerID, got, tt.expected)
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

	// We expect 5 agent types x 6 providers = 30 definitions.
	// (architect, eval-designer, context-gatherer, judge, planner)
	if count != 30 {
		t.Errorf("expected 30 YAML definitions, got %d", count)
	}
}
