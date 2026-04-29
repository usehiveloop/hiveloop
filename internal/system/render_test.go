package system

import (
	"strings"
	"testing"
)

// TestRenderUserPrompt_Substitutes verifies that user prompt templates are correctly
// substituted with provided values.
func TestRenderUserPrompt_Substitutes(t *testing.T) {
	task := Task{
		Name:               "x",
		UserPromptTemplate: "Shape: {{.shape}}\nGoal: {{.goal}}",
	}
	got, err := RenderUserPrompt(task, map[string]any{
		"shape": "haiku",
		"goal":  "delight",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(got, "Shape: haiku") || !strings.Contains(got, "Goal: delight") {
		t.Fatalf("substitution failed: %q", got)
	}
}

// TestRenderUserPrompt_MissingKeyIsError verifies that missing keys cause an error.
func TestRenderUserPrompt_MissingKeyIsError(t *testing.T) {
	// missingkey=error means a referenced var must be present in the map.
	task := Task{Name: "x", UserPromptTemplate: "Hi {{.who}}"}
	_, err := RenderUserPrompt(task, map[string]any{})
	if err == nil {
		t.Fatalf("expected missing-key error, got nil")
	}
}

// Note: Tests for BuildLLMRequest variants were removed as they test framework
// behavior rather than business logic. The remaining tests verify actual prompt
// substitution which is business logic.
// See USELESS_TESTS_RECOMMENDATIONS.md for details.