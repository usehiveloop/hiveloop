package main

import (
	"encoding/json"
	"strings"
)

// ActionDef mirrors the catalog ActionDef for JSON output.
type ActionDef struct {
	DisplayName  string           `json:"display_name"`
	Description  string           `json:"description"`
	Access       string           `json:"access"`
	ResourceType string           `json:"resource_type"`
	Parameters   json.RawMessage  `json:"parameters"`
	Execution    *ExecutionConfig `json:"execution,omitempty"`
}

// ExecutionConfig for GraphQL actions.
type ExecutionConfig struct {
	Method           string            `json:"method"`
	Path             string            `json:"path"`
	BodyMapping      map[string]string `json:"body_mapping,omitempty"`
	QueryMapping     map[string]string `json:"query_mapping,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	ResponsePath     string            `json:"response_path,omitempty"`
	GraphQLOperation string            `json:"graphql_operation,omitempty"` // "query" or "mutation"
	GraphQLField     string            `json:"graphql_field,omitempty"`     // top-level field name
}

// jsonSchemaProperty represents a single property in the JSON Schema.
type jsonSchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// parseSchema parses an introspected GraphQL schema into ActionDef entries.
func parseSchema(schema *IntrospectionSchema, cfg ServiceConfig) map[string]ActionDef {
	actions := make(map[string]ActionDef)

	// Build a type map for quick lookup.
	typeMap := make(map[string]*IntrospectType, len(schema.Types))
	for i := range schema.Types {
		typeMap[schema.Types[i].Name] = &schema.Types[i]
	}

	// Process Query type fields.
	if schema.QueryType != nil {
		if qt, ok := typeMap[schema.QueryType.Name]; ok {
			for _, field := range qt.Fields {
				if shouldSkipField(field.Name) {
					continue
				}
				if !matchesFieldFilters(field.Name, cfg.QueryFilters) {
					continue
				}

				actionKey := toSnakeCase(field.Name)
				displayName := toDisplayName(actionKey)

				desc := ""
				if field.Description != "" {
					desc = truncateDescription(field.Description, 200)
				}

				params, required := buildGraphQLParams(field.Args)

				schema := map[string]any{
					"type":       "object",
					"properties": params,
				}
				if len(required) > 0 {
					schema["required"] = required
				}
				paramsJSON, _ := json.Marshal(schema)

				actions[actionKey] = ActionDef{
					DisplayName:  displayName,
					Description:  desc,
					Access:       "read",
					ResourceType: "",
					Parameters:   paramsJSON,
					Execution: &ExecutionConfig{
						Method:           "POST",
						Path:             "/graphql",
						GraphQLOperation: "query",
						GraphQLField:     field.Name,
					},
				}
			}
		}
	}

	// Process Mutation type fields.
	if schema.MutationType != nil {
		if mt, ok := typeMap[schema.MutationType.Name]; ok {
			for _, field := range mt.Fields {
				if shouldSkipField(field.Name) {
					continue
				}
				if !matchesFieldFilters(field.Name, cfg.MutationFilters) {
					continue
				}

				actionKey := toSnakeCase(field.Name)
				// Prefix with "mutation_" if it collides with a query.
				if _, exists := actions[actionKey]; exists {
					actionKey = "mutation_" + actionKey
				}

				displayName := toDisplayName(actionKey)

				desc := ""
				if field.Description != "" {
					desc = truncateDescription(field.Description, 200)
				}

				params, required := buildGraphQLParams(field.Args)

				schema := map[string]any{
					"type":       "object",
					"properties": params,
				}
				if len(required) > 0 {
					schema["required"] = required
				}
				paramsJSON, _ := json.Marshal(schema)

				actions[actionKey] = ActionDef{
					DisplayName:  displayName,
					Description:  desc,
					Access:       "write",
					ResourceType: "",
					Parameters:   paramsJSON,
					Execution: &ExecutionConfig{
						Method:           "POST",
						Path:             "/graphql",
						GraphQLOperation: "mutation",
						GraphQLField:     field.Name,
					},
				}
			}
		}
	}

	return actions
}

// shouldSkipField returns true for internal/meta fields.
func shouldSkipField(name string) bool {
	return strings.HasPrefix(name, "__") || name == "node" || name == "nodes"
}

// matchesFieldFilters checks if a field name matches any of the prefix filters.
func matchesFieldFilters(name string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	nameLower := strings.ToLower(name)
	for _, f := range filters {
		if strings.HasPrefix(nameLower, strings.ToLower(f)) {
			return true
		}
	}
	return false
}

// parseSDLToActions converts a parsed SDL schema into ActionDef entries.
func parseSDLToActions(schema *sdlSchema, cfg ServiceConfig) map[string]ActionDef {
	actions := make(map[string]ActionDef)

	// Process query fields → read actions.
	for _, field := range schema.QueryFields {
		if shouldSkipField(field.Name) {
			continue
		}
		if !matchesFieldFilters(field.Name, cfg.QueryFilters) {
			continue
		}

		actionKey := toSnakeCase(field.Name)
		displayName := toDisplayName(actionKey)

		desc := ""
		if field.Description != "" {
			desc = truncateDescription(field.Description, 200)
		}

		params, required := buildSDLParams(field.Args)
		paramSchema := map[string]any{
			"type":       "object",
			"properties": params,
		}
		if len(required) > 0 {
			paramSchema["required"] = required
		}
		paramsJSON, _ := json.Marshal(paramSchema)

		actions[actionKey] = ActionDef{
			DisplayName:  displayName,
			Description:  desc,
			Access:       "read",
			ResourceType: "",
			Parameters:   paramsJSON,
			Execution: &ExecutionConfig{
				Method:           "POST",
				Path:             "/graphql",
				GraphQLOperation: "query",
				GraphQLField:     field.Name,
			},
		}
	}

	// Process mutation fields → write actions.
	for _, field := range schema.MutationFields {
		if shouldSkipField(field.Name) {
			continue
		}
		if !matchesFieldFilters(field.Name, cfg.MutationFilters) {
			continue
		}

		actionKey := toSnakeCase(field.Name)
		if _, exists := actions[actionKey]; exists {
			actionKey = "mutation_" + actionKey
		}

		displayName := toDisplayName(actionKey)

		desc := ""
		if field.Description != "" {
			desc = truncateDescription(field.Description, 200)
		}

		params, required := buildSDLParams(field.Args)
		paramSchema := map[string]any{
			"type":       "object",
			"properties": params,
		}
		if len(required) > 0 {
			paramSchema["required"] = required
		}
		paramsJSON, _ := json.Marshal(paramSchema)

		actions[actionKey] = ActionDef{
			DisplayName:  displayName,
			Description:  desc,
			Access:       "write",
			ResourceType: "",
			Parameters:   paramsJSON,
			Execution: &ExecutionConfig{
				Method:           "POST",
				Path:             "/graphql",
				GraphQLOperation: "mutation",
				GraphQLField:     field.Name,
			},
		}
	}

	return actions
}

// buildSDLParams converts SDL arguments to JSON Schema properties.
func buildSDLParams(args []sdlArg) (map[string]jsonSchemaProperty, []string) {
	properties := make(map[string]jsonSchemaProperty)
	var required []string

	for _, arg := range args {
		desc := arg.Description
		if len(desc) > 200 {
			desc = desc[:197] + "..."
		}

		properties[arg.Name] = jsonSchemaProperty{
			Type:        sdlTypeToJSONSchema(arg.Type),
			Description: desc,
		}

		if arg.Required {
			required = append(required, arg.Name)
		}
	}

	return properties, required
}

// buildGraphQLParams converts GraphQL arguments to JSON Schema properties.
func buildGraphQLParams(args []IntrospectInput) (map[string]jsonSchemaProperty, []string) {
	properties := make(map[string]jsonSchemaProperty)
	var required []string

	for _, arg := range args {
		desc := arg.Description
		if len(desc) > 200 {
			desc = desc[:197] + "..."
		}

		properties[arg.Name] = jsonSchemaProperty{
			Type:        graphqlTypeToJSONSchema(arg.Type),
			Description: desc,
		}

		if isNonNull(arg.Type) {
			required = append(required, arg.Name)
		}
	}

	return properties, required
}
