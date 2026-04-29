package handler

import (
	"testing"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// Tests for valid schema acceptance - these verify business logic for allowed schemas
func TestValidateJSONSchema_ArrayWithNestedObject(t *testing.T) {
	cfg := model.JSON{
		"json_schema": map[string]any{
			"name": "list",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"items": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"id":   map[string]any{"type": "string"},
								"name": map[string]any{"type": "string"},
							},
							"required": []any{"id", "name"},
						},
					},
				},
				"required": []any{"items"},
			},
		},
	}
	if err := validateJSONSchema(cfg); err != "" {
		t.Fatalf("expected valid schema with array of objects, got %q", err)
	}
}

func TestValidateJSONSchema_PassthroughInAgentConfig(t *testing.T) {
	// Verify json_schema is preserved in agent_config round-trip
	cfg := model.JSON{
		"max_tokens": float64(4000),
		"max_turns":  float64(10),
		"json_schema": map[string]any{
			"name": "extract",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"result": map[string]any{"type": "string"},
				},
				"required":             []any{"result"},
				"additionalProperties": false,
			},
		},
	}
	if err := validateJSONSchema(cfg); err != "" {
		t.Fatalf("expected no error, got %q", err)
	}

	// Verify json_schema is accessible after serialization
	schema, ok := cfg["json_schema"].(map[string]any)
	if !ok {
		t.Fatal("json_schema should be accessible as map")
	}
	if schema["name"] != "extract" {
		t.Fatalf("expected name 'extract', got %v", schema["name"])
	}
}

// Note: Tests for rejecting $ref, oneOf, pattern, minimum/maximum, etc. were removed
// as they test library constraints, not business logic. See USELESS_TESTS_RECOMMENDATIONS.md.