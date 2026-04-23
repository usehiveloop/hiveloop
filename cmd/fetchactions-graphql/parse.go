package main

import (
	"encoding/json"
	"sort"
	"strings"
)

// parseSchema parses an introspected GraphQL schema into ActionDef entries.
func parseSchema(schema *IntrospectionSchema, cfg ServiceConfig) (map[string]ActionDef, map[string]SchemaDefinition) {
	actions := make(map[string]ActionDef)
	includeSet := buildIncludeSet(cfg)

	typeMap := make(map[string]*IntrospectType, len(schema.Types))
	for idx := range schema.Types {
		typeMap[schema.Types[idx].Name] = &schema.Types[idx]
	}

	inputTypeFields := make(map[string][]IntrospectInput)
	for _, introType := range schema.Types {
		if introType.Kind == "INPUT_OBJECT" && len(introType.InputFields) > 0 {
			inputTypeFields[introType.Name] = introType.InputFields
		}
	}

	if schema.QueryType != nil {
		if queryType, ok := typeMap[schema.QueryType.Name]; ok {
			for _, field := range queryType.Fields {
				if shouldSkipField(field.Name) {
					continue
				}
				if !isFieldIncluded(field.Name, includeSet, cfg.QueryFilters) {
					continue
				}

				actionKey := toSnakeCase(field.Name)
				params, required := buildIntrospectionParams(field.Args, inputTypeFields)
				paramsJSON := marshalParamSchema(params, required)
				returnType := getNamedType(field.Type)
				resSchema := responseSchemaKey(returnType)

				actions[actionKey] = ActionDef{
					DisplayName:    toDisplayName(actionKey),
					Description:    truncateDescription(field.Description, 200),
					Access:         "read",
					ResourceType:   inferResourceType(field.Name, cfg.ResourcePrefixes),
					Parameters:     paramsJSON,
					ResponseSchema: resSchema,
					Execution: &ExecConfig{
						Method:           "POST",
						Path:             "/graphql",
						BodyMapping:      buildIntrospectionBodyMapping(field.Args),
						GraphQLOperation: "query",
						GraphQLField:     field.Name,
					},
				}
			}
		}
	}

	if schema.MutationType != nil {
		if mutationType, ok := typeMap[schema.MutationType.Name]; ok {
			for _, field := range mutationType.Fields {
				if shouldSkipField(field.Name) {
					continue
				}
				if !isFieldIncluded(field.Name, includeSet, cfg.MutationFilters) {
					continue
				}

				actionKey := toSnakeCase(field.Name)
				if _, exists := actions[actionKey]; exists {
					actionKey = "mutation_" + actionKey
				}

				params, required := buildIntrospectionParams(field.Args, inputTypeFields)
				paramsJSON := marshalParamSchema(params, required)
				returnType := getNamedType(field.Type)
				resSchema := responseSchemaKey(returnType)

				actions[actionKey] = ActionDef{
					DisplayName:    toDisplayName(actionKey),
					Description:    truncateDescription(field.Description, 200),
					Access:         "write",
					ResourceType:   inferResourceType(field.Name, cfg.ResourcePrefixes),
					Parameters:     paramsJSON,
					ResponseSchema: resSchema,
					Execution: &ExecConfig{
						Method:           "POST",
						Path:             "/graphql",
						BodyMapping:      buildIntrospectionBodyMapping(field.Args),
						GraphQLOperation: "mutation",
						GraphQLField:     field.Name,
					},
				}
			}
		}
	}

	schemas := buildIntrospectionSchemas(actions, typeMap)

	return actions, schemas
}

// parseSDLToActions converts a parsed SDL schema into ActionDef entries and response schemas.
func parseSDLToActions(schema *sdlSchema, cfg ServiceConfig) (map[string]ActionDef, map[string]SchemaDefinition) {
	actions := make(map[string]ActionDef)
	includeSet := buildIncludeSet(cfg)

	for _, field := range schema.QueryFields {
		if shouldSkipField(field.Name) {
			continue
		}
		if !isFieldIncluded(field.Name, includeSet, cfg.QueryFilters) {
			continue
		}

		actionKey := toSnakeCase(field.Name)
		params, required := buildSDLParams(field.Args, schema.InputTypes)
		paramsJSON := marshalParamSchema(params, required)
		resSchema := responseSchemaKey(baseTypeName(field.Type))

		actions[actionKey] = ActionDef{
			DisplayName:    toDisplayName(actionKey),
			Description:    truncateDescription(field.Description, 200),
			Access:         "read",
			ResourceType:   inferResourceType(field.Name, cfg.ResourcePrefixes),
			Parameters:     paramsJSON,
			ResponseSchema: resSchema,
			Execution: &ExecConfig{
				Method:           "POST",
				Path:             "/graphql",
				BodyMapping:      buildSDLBodyMapping(field.Args),
				GraphQLOperation: "query",
				GraphQLField:     field.Name,
			},
		}
	}

	for _, field := range schema.MutationFields {
		if shouldSkipField(field.Name) {
			continue
		}
		if !isFieldIncluded(field.Name, includeSet, cfg.MutationFilters) {
			continue
		}

		actionKey := toSnakeCase(field.Name)
		if _, exists := actions[actionKey]; exists {
			actionKey = "mutation_" + actionKey
		}

		params, required := buildSDLParams(field.Args, schema.InputTypes)
		paramsJSON := marshalParamSchema(params, required)
		resSchema := responseSchemaKey(baseTypeName(field.Type))

		actions[actionKey] = ActionDef{
			DisplayName:    toDisplayName(actionKey),
			Description:    truncateDescription(field.Description, 200),
			Access:         "write",
			ResourceType:   inferResourceType(field.Name, cfg.ResourcePrefixes),
			Parameters:     paramsJSON,
			ResponseSchema: resSchema,
			Execution: &ExecConfig{
				Method:           "POST",
				Path:             "/graphql",
				BodyMapping:      buildSDLBodyMapping(field.Args),
				GraphQLOperation: "mutation",
				GraphQLField:     field.Name,
			},
		}
	}

	schemas := buildSDLSchemas(actions, schema.ObjectTypes)

	return actions, schemas
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
	for _, filterPrefix := range filters {
		if strings.HasPrefix(nameLower, strings.ToLower(filterPrefix)) {
			return true
		}
	}
	return false
}

// buildIncludeSet converts the IncludeFields slice into a fast-lookup set.
func buildIncludeSet(cfg ServiceConfig) map[string]bool {
	if len(cfg.IncludeFields) == 0 {
		return nil
	}
	includeSet := make(map[string]bool, len(cfg.IncludeFields))
	for _, fieldName := range cfg.IncludeFields {
		includeSet[fieldName] = true
	}
	return includeSet
}

// isFieldIncluded checks if a field should be included.
func isFieldIncluded(fieldName string, includeSet map[string]bool, prefixFilters []string) bool {
	if includeSet != nil {
		return includeSet[fieldName]
	}
	return matchesFieldFilters(fieldName, prefixFilters)
}

// buildSDLBodyMapping creates a body_mapping from SDL args.
func buildSDLBodyMapping(args []sdlArg) map[string]string {
	if len(args) == 0 {
		return nil
	}
	mapping := make(map[string]string, len(args))
	for _, arg := range args {
		mapping[arg.Name] = arg.Name
	}
	return mapping
}

// buildIntrospectionBodyMapping creates a body_mapping from introspection args.
func buildIntrospectionBodyMapping(args []IntrospectInput) map[string]string {
	if len(args) == 0 {
		return nil
	}
	mapping := make(map[string]string, len(args))
	for _, arg := range args {
		mapping[arg.Name] = arg.Name
	}
	return mapping
}

// marshalParamSchema builds the JSON Schema object for parameters.
func marshalParamSchema(properties map[string]jsonSchemaProperty, required []string) json.RawMessage {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	data, _ := json.Marshal(schema)
	return data
}
