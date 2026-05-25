package evals

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSuiteRequiresBusinessAndMemories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "suite.yaml")
	body := []byte(`
id: delegation
business:
  name: Luma Cakes
  profile: Real business profile.
memories:
  - type: fact
    content: The owner likes concise updates.
cases:
  - id: research
    message: Can you look into this market?
    expected_behavior: delegate
    expected_specialist: business-research-specialist
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}
	suite, err := LoadSuite(path)
	if err != nil {
		t.Fatalf("LoadSuite: %v", err)
	}
	if suite.Business.Name != "Luma Cakes" || len(suite.Memories) != 1 {
		t.Fatalf("suite fixture not loaded: %#v", suite)
	}
}

func TestDefaultDelegationSuiteLoads(t *testing.T) {
	suite, err := LoadSuite("../../evals/employee-delegation-v1.yaml")
	if err != nil {
		t.Fatalf("LoadSuite default: %v", err)
	}
	if len(suite.Cases) < 4 {
		t.Fatalf("default suite cases = %d, want at least 4", len(suite.Cases))
	}
}

func TestLoadSuiteRejectsDelegateWithoutSpecialist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "suite.yaml")
	body := []byte(`
id: delegation
business:
  name: Luma Cakes
  profile: Real business profile.
memories:
  - type: fact
    content: The owner likes concise updates.
cases:
  - id: research
    message: Can you look into this market?
    expected_behavior: delegate
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}
	if _, err := LoadSuite(path); err == nil {
		t.Fatal("LoadSuite succeeded without expected specialist")
	}
}
