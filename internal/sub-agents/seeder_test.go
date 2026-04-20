package subagents

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAllYAMLsParse(t *testing.T) {
	typeDirs, err := subagentsFS.ReadDir(".")
	if err != nil {
		t.Fatalf("read root: %v", err)
	}

	count := 0
	dirCount := 0
	for _, typeDir := range typeDirs {
		if !typeDir.IsDir() {
			continue
		}
		dirCount++

		files, err := subagentsFS.ReadDir(typeDir.Name())
		if err != nil {
			t.Fatalf("read %s: %v", typeDir.Name(), err)
		}

		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".yaml") {
				continue
			}

			path := filepath.Join(typeDir.Name(), file.Name())
			data, err := subagentsFS.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			var sf subagentFile
			if err := yaml.Unmarshal(data, &sf); err != nil {
				t.Errorf("%s: YAML parse error: %v", path, err)
				continue
			}
			if sf.Model == "" {
				t.Errorf("%s: model is empty", path)
			}
			if sf.SystemPrompt == "" {
				t.Errorf("%s: system_prompt is empty", path)
			}
			count++
		}
	}

	if count < 42 {
		t.Errorf("expected at least 42 YAML definitions, got %d", count)
	}
	t.Logf("parsed %d YAML definitions across %d subagent types", count, dirCount)
}

func TestLoadGroupMergesProviders(t *testing.T) {
	group, err := loadGroup("browser-tester-expert")
	if err != nil {
		t.Fatalf("loadGroup: %v", err)
	}

	if group.name != "browser-expert" {
		t.Errorf("expected name browser-expert, got %s", group.name)
	}

	expectedProviders := []string{"anthropic", "gemini", "glm", "kimi", "minimax", "openai"}
	for _, provider := range expectedProviders {
		if _, ok := group.providers[provider]; !ok {
			t.Errorf("missing provider %s", provider)
		}
	}

	if len(group.providers) != 6 {
		t.Errorf("expected 6 providers, got %d", len(group.providers))
	}
}
