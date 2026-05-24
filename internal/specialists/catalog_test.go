package specialists

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGlobalSpecialists(t *testing.T) {
	catalog, err := Load("../../global/specialists")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	items := catalog.List()
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}
	def, ok := catalog.BySlug("software-engineering-specialist")
	if !ok {
		t.Fatalf("software engineering specialist not found")
	}
	if !def.AutoAttach {
		t.Fatalf("AutoAttach = false, want true")
	}
	if !strings.Contains(def.SystemPrompt, "Software Engineering Specialist") {
		t.Fatalf("SystemPrompt does not contain migrated prompt")
	}
}

func TestLoadRejectsDuplicateSlug(t *testing.T) {
	dir := t.TempDir()
	writeSpecialist(t, filepath.Join(dir, "one"), "duplicate")
	writeSpecialist(t, filepath.Join(dir, "two"), "duplicate")

	if _, err := Load(dir); err == nil {
		t.Fatalf("Load() error = nil, want duplicate slug error")
	}
}

func TestLoadRejectsMissingPrompt(t *testing.T) {
	dir := t.TempDir()
	specialistDir := filepath.Join(dir, "bad")
	if err := os.MkdirAll(specialistDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specialistDir, "agent.json"), []byte(`{
  "slug":"bad",
  "name":"Bad",
  "description":"Bad specialist",
  "specialist_type":"bad",
  "version":1,
  "prompt_path":"missing.md"
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(dir); err == nil {
		t.Fatalf("Load() error = nil, want missing prompt error")
	}
}

func writeSpecialist(t *testing.T, dir string, slug string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.json"), []byte(`{
  "slug":"`+slug+`",
  "name":"Test",
  "description":"Test specialist",
  "specialist_type":"test",
  "version":1,
  "prompt_path":"prompt.md"
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("You are a test specialist."), 0o644); err != nil {
		t.Fatal(err)
	}
}
