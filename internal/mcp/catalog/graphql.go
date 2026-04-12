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
// Scalar fields are included directly. Object fields get a nested { id name }
// selection (safe default for most entities). Arrays are skipped to keep
// queries lightweight — the enrichment LLM can plan separate fetches for
// related collections if needed.
//
// maxDepth prevents infinite recursion on self-referential schemas.
func buildSelectionSet(schema SchemaDefinition, allSchemas map[string]SchemaDefinition, depth int) string {
	if depth > 1 || len(schema.Properties) == 0 {
		return "{ id }"
	}

	var fields []string
	for fieldName, prop := range schema.Properties {
		fieldType := prop.Type
		if fieldType == "" && prop.SchemaRef != "" {
			fieldType = "ref"
		}

		switch fieldType {
		case "string", "number", "integer", "boolean":
			fields = append(fields, fieldName)
		case "object":
			// Nested object: select id + name as safe defaults.
			// These are the two most universally present fields on Linear entities.
			fields = append(fields, fieldName+" { id name }")
		case "array":
			// Skip arrays in automatic selection — they can be huge (e.g., labels, comments).
			// The enrichment LLM plans separate fetches for collections if needed.
		case "ref":
			// Schema reference — resolve and recurse (one level).
			// Not currently used by Linear schemas (they use inline type: "object")
			// but included for completeness.
			fields = append(fields, fieldName+" { id name }")
		default:
			// Unknown type — include as scalar (will be null if not a real field).
			fields = append(fields, fieldName)
		}
	}

	if len(fields) == 0 {
		return "{ id }"
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
