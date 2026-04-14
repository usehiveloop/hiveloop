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
	for _, typeDir := range typeDirs {
		if !typeDir.IsDir() {
			continue
		}

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

	if count != 42 {
		t.Errorf("expected 42 YAML definitions (7 subagents x 6 providers), got %d", count)
	}
}
