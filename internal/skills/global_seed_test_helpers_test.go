package skills_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeGlobalSkill(t *testing.T, root, name, description, content string, files []map[string]string) {
	t.Helper()
	skillDir := filepath.Join(root, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := map[string]any{
		"name":        name,
		"description": description,
		"root":        "./SKILL.md",
		"files":       files,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), body, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func skillsForRepoTest() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return "", os.ErrNotExist
		}
		cwd = parent
	}
}
