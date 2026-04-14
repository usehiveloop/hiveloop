package catalog

import (
	"fmt"
	"strings"
)

// BuildGraphQLRequest constructs a GraphQL query or mutation request body
// from an action's execution config and its response schema. Used by the
// executor when running enrichment steps against GraphQL providers (e.g. Linear).
//
// Returns the JSON body to POST to /graphql:
//
//	{"query": "query { issue(id: \"abc\") { id title ... } }"}
//
// For mutations:
//
//	{"query": "mutation { issueCreate(input: {...}) { success issue { id } } }"}
func BuildGraphQLRequest(action ActionDef, execConfig ExecutionConfig, params map[string]any, schemas map[string]SchemaDefinition) map[string]any {
	if execConfig.GraphQLField == "" {
		return nil
	}

	operation := execConfig.GraphQLOperation
	if operation == "" {
		operation = "query"
	}

	// Build argument string from body_mapping + params.
	argString := buildGraphQLArgs(execConfig.BodyMapping, params)

	// Build selection set from response schema.
	selectionSet := ""
	if action.ResponseSchema != "" {
		if schema, ok := schemas[action.ResponseSchema]; ok {
			selectionSet = buildSelectionSet(schema, schemas, 0)
		}
	}

	// For mutations, the response is typically a payload wrapper.
	// Linear mutations return {success, entity { ... }}.
	if operation == "mutation" && selectionSet == "" {
		selectionSet = "{ success }"
	}

	var query string
	if argString != "" {
		query = fmt.Sprintf("%s { %s(%s) %s }", operation, execConfig.GraphQLField, argString, selectionSet)
	} else {
		query = fmt.Sprintf("%s { %s %s }", operation, execConfig.GraphQLField, selectionSet)
	}

	return map[string]any{"query": query}
}

// IsGraphQL returns true if the execution config targets a GraphQL endpoint.
func IsGraphQL(execConfig ExecutionConfig) bool {
	return execConfig.GraphQLField != "" || execConfig.GraphQLOperation != ""
}

// buildGraphQLArgs constructs the argument string for a GraphQL field.
// E.g., body_mapping {"id": "id"} + params {"id": "abc"} → `id: "abc"`
func buildGraphQLArgs(bodyMapping map[string]string, params map[string]any) string {
	if len(bodyMapping) == 0 || len(params) == 0 {
		return ""
	}

	var args []string
	for paramName, graphqlArgName := range bodyMapping {
		value, ok := params[paramName]
		if !ok {
			continue
		}
		args = append(args, fmt.Sprintf("%s: %s", graphqlArgName, formatGraphQLValue(value)))
	}
	return strings.Join(args, ", ")
}

// buildSelectionSet generates a GraphQL selection set from a schema definition.
// Scalar fields are included directly. Object/ref fields resolve their schema_ref
// (if present) and recurse to select actual scalar fields from the referenced
// schema. Arrays of objects also resolve their schema_ref. This makes selection
// sets correct for any provider, not just Linear.
//
// maxDepth prevents infinite recursion on self-referential schemas.
func buildSelectionSet(schema SchemaDefinition, allSchemas map[string]SchemaDefinition, depth int) string {
	if depth > 3 || len(schema.Properties) == 0 {
		return ""
	}

	var fields []string
	for fieldName, prop := range schema.Properties {
		// If there's a schema_ref, resolve it regardless of type.
		if prop.SchemaRef != "" {
			if refSchema, ok := allSchemas[prop.SchemaRef]; ok {
				nested := buildSelectionSet(refSchema, allSchemas, depth+1)
				if nested != "" {
					fields = append(fields, fieldName+" "+nested)
				}
				continue
			}
		}

		switch prop.Type {
		case "string", "number", "integer", "boolean":
			fields = append(fields, fieldName)
		case "object":
			// Inline object without schema_ref — skip (can't know the fields).
		case "array":
			// Arrays without schema_ref — skip.
		default:
			fields = append(fields, fieldName)
		}
	}

	if len(fields) == 0 {
		return ""
	}

	return "{ " + strings.Join(fields, " ") + " }"
}

// formatGraphQLValue formats a Go value for inline GraphQL argument syntax.
func formatGraphQLValue(value any) string {
	switch typedValue := value.(type) {
	case string:
		// Escape quotes in string values.
		escaped := strings.ReplaceAll(typedValue, `"`, `\"`)
		return fmt.Sprintf(`"%s"`, escaped)
	case float64:
		if typedValue == float64(int(typedValue)) {
			return fmt.Sprintf("%d", int(typedValue))
		}
		return fmt.Sprintf("%g", typedValue)
	case int:
		return fmt.Sprintf("%d", typedValue)
	case bool:
		if typedValue {
			return "true"
		}
		return "false"
	case map[string]any:
		// Nested object (e.g., mutation input) — format as GraphQL object literal.
		var pairs []string
		for key, val := range typedValue {
			pairs = append(pairs, fmt.Sprintf("%s: %s", key, formatGraphQLValue(val)))
		}
		return "{ " + strings.Join(pairs, ", ") + " }"
	default:
		return fmt.Sprintf(`"%v"`, value)
	}
}
