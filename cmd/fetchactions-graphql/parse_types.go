package main

import (
	"encoding/json"
)

// ActionDef mirrors the catalog ActionDef for JSON output.
type ActionDef struct {
	DisplayName    string          `json:"display_name"`
	Description    string          `json:"description"`
	Access         string          `json:"access"`
	ResourceType   string          `json:"resource_type"`
	Parameters     json.RawMessage `json:"parameters"`
	Execution      *ExecConfig     `json:"execution,omitempty"`
	ResponseSchema string          `json:"response_schema,omitempty"`
}

// ExecConfig for GraphQL actions.
type ExecConfig struct {
	Method           string            `json:"method"`
	Path             string            `json:"path"`
	BodyMapping      map[string]string `json:"body_mapping,omitempty"`
	QueryMapping     map[string]string `json:"query_mapping,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	ResponsePath     string            `json:"response_path,omitempty"`
	GraphQLOperation string            `json:"graphql_operation,omitempty"`
	GraphQLField     string            `json:"graphql_field,omitempty"`
}

// jsonSchemaProperty represents a property in the JSON Schema, supporting nested objects.
type jsonSchemaProperty struct {
	Type        string                        `json:"type"`
	Description string                        `json:"description,omitempty"`
	Nullable    bool                          `json:"nullable,omitempty"`
	Properties  map[string]jsonSchemaProperty `json:"properties,omitempty"`
	Required    []string                      `json:"required,omitempty"`
}

// SchemaDefinition is a response type schema for the output file.
type SchemaDefinition struct {
	Type       string                       `json:"type"`
	Properties map[string]SchemaPropertyDef `json:"properties,omitempty"`
}

// SchemaPropertyDef describes a property in a response schema.
type SchemaPropertyDef struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Nullable    bool   `json:"nullable,omitempty"`
	SchemaRef   string `json:"schema_ref,omitempty"`
}
