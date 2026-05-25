package main

import (
	"encoding/json"
)

// ActionDef mirrors the catalog ActionDef for JSON output.
type ActionDef struct {
	DisplayName    string           `json:"display_name"`
	Description    string           `json:"description"`
	Access         string           `json:"access"`
	ResourceType   string           `json:"resource_type"`
	Parameters     json.RawMessage  `json:"parameters"`
	Execution      *ExecutionConfig `json:"execution,omitempty"`
	ResponseSchema string           `json:"response_schema,omitempty"`
}

// ExecutionConfig mirrors the catalog ExecutionConfig.
type ExecutionConfig struct {
	Method       string            `json:"method"`
	Path         string            `json:"path"`
	BodyMapping  map[string]string `json:"body_mapping,omitempty"`
	QueryMapping map[string]string `json:"query_mapping,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	ResponsePath string            `json:"response_path,omitempty"`
}

// jsonSchemaProperty represents a single property in the JSON Schema.
type jsonSchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// SchemaProperty describes a single property in a flattened response schema.
type SchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Nullable    bool   `json:"nullable,omitempty"`
	SchemaRef   string `json:"schema_ref,omitempty"`
}

// FlatSchema is a flattened top-level-only representation of a response schema.
type FlatSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]SchemaProperty `json:"properties,omitempty"`
	Items      *FlatSchemaRef            `json:"items,omitempty"`
}

// FlatSchemaRef references another schema by name (for array item types).
type FlatSchemaRef struct {
	Ref string `json:"$ref,omitempty"`
}

// ParseResult holds parsed actions and the referenced response schemas.
type ParseResult struct {
	Actions map[string]ActionDef
	Schemas map[string]FlatSchema
}
