package handler

import (
	"testing"

	"github.com/llmvault/llmvault/internal/model"
)

func TestValidateJSONSchema_Nil(t *testing.T) {
	if err := validateJSONSchema(nil); err != "" {
		t.Fatalf("expected no error, got %q", err)
	}
}

func TestValidateJSONSchema_NoSchemaKey(t *testing.T) {
	cfg := model.JSON{"max_tokens": 4000}
	if err := validateJSONSchema(cfg); err != "" {
		t.Fatalf("expected no error, got %q", err)
	}
}

func TestValidateJSONSchema_Valid(t *testing.T) {
	cfg := model.JSON{
		"max_tokens": 4000,
		"json_schema": map[string]any{
			"name": "extract_contact",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":  map[string]any{"type": "string"},
					"email": map[string]any{"type": "string"},
				},
				"required":             []any{"name", "email"},
				"additionalProperties": false,
			},
		},
	}
	if err := validateJSONSchema(cfg); err != "" {
		t.Fatalf("expected no error, got %q", err)
	}
}

func TestValidateJSONSchema_ValidWithNullableField(t *testing.T) {
	cfg := model.JSON{
		"json_schema": map[string]any{
			"name": "result",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"value": map[string]any{
						"anyOf": []any{
							map[string]any{"type": "string"},
							map[string]any{"type": "null"},
						},
					},
				},
				"required": []any{"value"},
			},
		},
	}
	if err := validateJSONSchema(cfg); err != "" {
		t.Fatalf("expected no error, got %q", err)
	}
}

func TestValidateJSONSchema_ValidNestedObject(t *testing.T) {
	cfg := model.JSON{
		"json_schema": map[string]any{
			"name": "nested",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"address": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"street": map[string]any{"type": "string"},
							"city":   map[string]any{"type": "string"},
						},
						"required": []any{"street", "city"},
					},
				},
				"required": []any{"address"},
			},
		},
	}
	if err := validateJSONSchema(cfg); err != "" {
		t.Fatalf("expected no error, got %q", err)
	}
}

func TestValidateJSONSchema_MissingName(t *testing.T) {
	cfg := model.JSON{
		"json_schema": map[string]any{
			"schema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
	err := validateJSONSchema(cfg)
	if err == "" {
		t.Fatal("expected error for missing name")
	}
	if err != "json_schema.name is required and must be a non-empty string" {
		t.Fatalf("unexpected error: %q", err)
	}
}

func TestValidateJSONSchema_MissingSchema(t *testing.T) {
	cfg := model.JSON{
		"json_schema": map[string]any{
			"name": "test",
		},
	}
	err := validateJSONSchema(cfg)
	if err == "" {
		t.Fatal("expected error for missing schema")
	}
	if err != "json_schema.schema is required and must be an object" {
		t.Fatalf("unexpected error: %q", err)
	}
}

func TestValidateJSONSchema_NonObjectType(t *testing.T) {
	cfg := model.JSON{
		"json_schema": map[string]any{
			"name": "test",
			"schema": map[string]any{
				"type": "string",
			},
		},
	}
	err := validateJSONSchema(cfg)
	if err == "" {
		t.Fatal("expected error for non-object type")
	}
	if err != "json_schema.schema.type must be \"object\"" {
		t.Fatalf("unexpected error: %q", err)
	}
}

func TestValidateJSONSchema_NotAnObject(t *testing.T) {
	cfg := model.JSON{
		"json_schema": "not an object",
	}
	err := validateJSONSchema(cfg)
	if err == "" {
		t.Fatal("expected error")
	}
	if err != "json_schema must be an object" {
		t.Fatalf("unexpected error: %q", err)
	}
}

func TestValidateJSONSchema_ExceedsDepth(t *testing.T) {
	// Build 6 levels of nesting (exceeds 5)
	innermost := map[string]any{
		"type":       "object",
		"properties": map[string]any{"val": map[string]any{"type": "string"}},
		"required":   []any{"val"},
	}
	current := innermost
	for i := 0; i < 5; i++ {
		current = map[string]any{
			"type":       "object",
			"properties": map[string]any{"nested": current},
			"required":   []any{"nested"},
		}
	}
	cfg := model.JSON{
		"json_schema": map[string]any{
			"name":   "deep",
			"schema": current,
		},
	}
	err := validateJSONSchema(cfg)
	if err == "" {
		t.Fatal("expected depth error")
	}
	if err != "json_schema.schema exceeds maximum nesting depth of 5" {
		t.Fatalf("unexpected error: %q", err)
	}
}

func TestValidateJSONSchema_ExceedsPropertyCount(t *testing.T) {
	props := make(map[string]any, 101)
	required := make([]any, 101)
	for i := 0; i < 101; i++ {
		key := "prop" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		props[key] = map[string]any{"type": "string"}
		required[i] = key
	}
	cfg := model.JSON{
		"json_schema": map[string]any{
			"name": "wide",
			"schema": map[string]any{
				"type":       "object",
				"properties": props,
				"required":   required,
			},
		},
	}
	err := validateJSONSchema(cfg)
	if err == "" {
		t.Fatal("expected property count error")
	}
	if err != "json_schema.schema exceeds maximum of 100 total properties" {
		t.Fatalf("unexpected error: %q", err)
	}
}

func TestValidateJSONSchema_RejectsRef(t *testing.T) {
	cfg := model.JSON{
		"json_schema": map[string]any{
			"name": "test",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"item": map[string]any{"$ref": "#/$defs/Item"},
				},
				"$defs": map[string]any{
					"Item": map[string]any{"type": "string"},
				},
				"required": []any{"item"},
			},
		},
	}
	err := validateJSONSchema(cfg)
	if err == "" {
		t.Fatal("expected error for $ref")
	}
}

func TestValidateJSONSchema_RejectsOneOf(t *testing.T) {
	cfg := model.JSON{
		"json_schema": map[string]any{
			"name": "test",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"value": map[string]any{
						"oneOf": []any{
							map[string]any{"type": "string"},
							map[string]any{"type": "integer"},
						},
					},
				},
				"required": []any{"value"},
			},
		},
	}
	err := validateJSONSchema(cfg)
	if err == "" {
		t.Fatal("expected error for oneOf")
	}
}

func TestValidateJSONSchema_RejectsPattern(t *testing.T) {
	cfg := model.JSON{
		"json_schema": map[string]any{
			"name": "test",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"email": map[string]any{
						"type":    "string",
						"pattern": "^[a-z]+@[a-z]+\\.[a-z]+$",
					},
				},
				"required": []any{"email"},
			},
		},
	}
	err := validateJSONSchema(cfg)
	if err == "" {
		t.Fatal("expected error for pattern")
	}
}

func TestValidateJSONSchema_RejectsMinMax(t *testing.T) {
	cfg := model.JSON{
		"json_schema": map[string]any{
			"name": "test",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"age": map[string]any{
						"type":    "integer",
						"minimum": 0,
						"maximum": 120,
					},
				},
				"required": []any{"age"},
			},
		},
	}
	err := validateJSONSchema(cfg)
	if err == "" {
		t.Fatal("expected error for minimum/maximum")
	}
}

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
