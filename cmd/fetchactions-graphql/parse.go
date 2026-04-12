package main

import (
	"encoding/json"
	"sort"
	"strings"
)

// ActionDef mirrors the catalog ActionDef for JSON output.
type ActionDef struct {
	DisplayName    string           `json:"display_name"`
	Description    string           `json:"description"`
	Access         string           `json:"access"`
	ResourceType   string           `json:"resource_type"`
	Parameters     json.RawMessage  `json:"parameters"`
	Execution      *ExecConfig      `json:"execution,omitempty"`
	ResponseSchema string           `json:"response_schema,omitempty"`
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
	Properties  map[string]jsonSchemaProperty  `json:"properties,omitempty"`
	Required    []string                       `json:"required,omitempty"`
}

// SchemaDefinition is a response type schema for the output file.
type SchemaDefinition struct {
	Type       string                         `json:"type"`
	Properties map[string]SchemaPropertyDef    `json:"properties,omitempty"`
}

// SchemaPropertyDef describes a property in a response schema.
type SchemaPropertyDef struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Nullable    bool   `json:"nullable,omitempty"`
	SchemaRef   string `json:"schema_ref,omitempty"`
}

// parseSchema parses an introspected GraphQL schema into ActionDef entries.
func parseSchema(schema *IntrospectionSchema, cfg ServiceConfig) (map[string]ActionDef, map[string]SchemaDefinition) {
	actions := make(map[string]ActionDef)
	includeSet := buildIncludeSet(cfg)

	// Build a type map for quick lookup.
	typeMap := make(map[string]*IntrospectType, len(schema.Types))
	for idx := range schema.Types {
		typeMap[schema.Types[idx].Name] = &schema.Types[idx]
	}

	// Build input type field map from introspected types for parameter expansion.
	inputTypeFields := make(map[string][]IntrospectInput)
	for _, introType := range schema.Types {
		if introType.Kind == "INPUT_OBJECT" && len(introType.InputFields) > 0 {
			inputTypeFields[introType.Name] = introType.InputFields
		}
	}

	// Process Query type fields.
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

	// Process Mutation type fields.
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

	// Build response schemas from object types in the introspection data.
	schemas := buildIntrospectionSchemas(actions, typeMap)

	return actions, schemas
}

// parseSDLToActions converts a parsed SDL schema into ActionDef entries and response schemas.
func parseSDLToActions(schema *sdlSchema, cfg ServiceConfig) (map[string]ActionDef, map[string]SchemaDefinition) {
	actions := make(map[string]ActionDef)
	includeSet := buildIncludeSet(cfg)

	// Process query fields → read actions.
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

	// Process mutation fields → write actions.
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

	// Build response schemas from object types.
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
// Returns nil if IncludeFields is empty (meaning "include all").
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
// If includeSet is nil (no allowlist), falls back to prefix filters.
func isFieldIncluded(fieldName string, includeSet map[string]bool, prefixFilters []string) bool {
	if includeSet != nil {
		return includeSet[fieldName]
	}
	return matchesFieldFilters(fieldName, prefixFilters)
}

// buildSDLBodyMapping creates a body_mapping from SDL args.
// Each GraphQL argument becomes a variable — maps arg name → arg name (1:1).
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

// buildSDLParams converts SDL arguments to JSON Schema properties,
// expanding input object types into nested object schemas.
func buildSDLParams(args []sdlArg, inputTypes map[string][]sdlField) (map[string]jsonSchemaProperty, []string) {
	properties := make(map[string]jsonSchemaProperty)
	var required []string

	for _, arg := range args {
		typeName := baseTypeName(arg.Type)

		// Check if this argument is an input object type.
		if inputFields, ok := inputTypes[typeName]; ok {
			// Expand the input type into a typed object with nested properties.
			nested := make(map[string]jsonSchemaProperty)
			var nestedRequired []string

			for _, inputField := range inputFields {
				fieldTypeName := baseTypeName(inputField.Type)

				// Check for nested input types (one level deep).
				if nestedInputFields, nestedOk := inputTypes[fieldTypeName]; nestedOk {
					innerProps := make(map[string]jsonSchemaProperty)
					var innerRequired []string
					for _, innerField := range nestedInputFields {
						innerProps[innerField.Name] = jsonSchemaProperty{
							Type:        sdlTypeToJSONSchema(innerField.Type),
							Description: truncateDescription(innerField.Description, 200),
						}
						if strings.HasSuffix(innerField.Type, "!") {
							innerRequired = append(innerRequired, innerField.Name)
						}
					}
					sort.Strings(innerRequired)
					prop := jsonSchemaProperty{
						Type:        "object",
						Description: truncateDescription(inputField.Description, 200),
						Properties:  innerProps,
					}
					if len(innerRequired) > 0 {
						prop.Required = innerRequired
					}
					nested[inputField.Name] = prop
				} else {
					nested[inputField.Name] = jsonSchemaProperty{
						Type:        sdlTypeToJSONSchema(inputField.Type),
						Description: truncateDescription(inputField.Description, 200),
					}
				}

				if strings.HasSuffix(inputField.Type, "!") {
					nestedRequired = append(nestedRequired, inputField.Name)
				}
			}

			sort.Strings(nestedRequired)
			prop := jsonSchemaProperty{
				Type:        "object",
				Description: truncateDescription(arg.Description, 200),
				Properties:  nested,
			}
			if len(nestedRequired) > 0 {
				prop.Required = nestedRequired
			}
			properties[arg.Name] = prop
		} else {
			// Scalar, enum, or unknown type.
			desc := arg.Description
			if len(desc) > 200 {
				desc = desc[:197] + "..."
			}
			properties[arg.Name] = jsonSchemaProperty{
				Type:        sdlTypeToJSONSchema(arg.Type),
				Description: desc,
			}
		}

		if arg.Required {
			required = append(required, arg.Name)
		}
	}

	return properties, required
}

// buildIntrospectionParams converts introspection arguments to JSON Schema properties,
// expanding input object types into nested object schemas.
func buildIntrospectionParams(args []IntrospectInput, inputTypeFields map[string][]IntrospectInput) (map[string]jsonSchemaProperty, []string) {
	properties := make(map[string]jsonSchemaProperty)
	var required []string

	for _, arg := range args {
		typeName := getNamedType(arg.Type)

		// Check if this argument is an input object type.
		if inputFields, ok := inputTypeFields[typeName]; ok {
			nested := make(map[string]jsonSchemaProperty)
			var nestedRequired []string

			for _, inputField := range inputFields {
				innerTypeName := getNamedType(inputField.Type)

				// Check for nested input types (one level deep).
				if nestedInputFields, nestedOk := inputTypeFields[innerTypeName]; nestedOk {
					innerProps := make(map[string]jsonSchemaProperty)
					var innerRequired []string
					for _, innerField := range nestedInputFields {
						innerProps[innerField.Name] = jsonSchemaProperty{
							Type:        graphqlTypeToJSONSchema(innerField.Type),
							Description: truncateDescription(innerField.Description, 200),
						}
						if isNonNull(innerField.Type) {
							innerRequired = append(innerRequired, innerField.Name)
						}
					}
					sort.Strings(innerRequired)
					prop := jsonSchemaProperty{
						Type:        "object",
						Description: truncateDescription(inputField.Description, 200),
						Properties:  innerProps,
					}
					if len(innerRequired) > 0 {
						prop.Required = innerRequired
					}
					nested[inputField.Name] = prop
				} else {
					nested[inputField.Name] = jsonSchemaProperty{
						Type:        graphqlTypeToJSONSchema(inputField.Type),
						Description: truncateDescription(inputField.Description, 200),
					}
				}

				if isNonNull(inputField.Type) {
					nestedRequired = append(nestedRequired, inputField.Name)
				}
			}

			sort.Strings(nestedRequired)
			prop := jsonSchemaProperty{
				Type:        "object",
				Description: truncateDescription(arg.Description, 200),
				Properties:  nested,
			}
			if len(nestedRequired) > 0 {
				prop.Required = nestedRequired
			}
			properties[arg.Name] = prop
		} else {
			properties[arg.Name] = jsonSchemaProperty{
				Type:        graphqlTypeToJSONSchema(arg.Type),
				Description: truncateDescription(arg.Description, 200),
			}
		}

		if isNonNull(arg.Type) {
			required = append(required, arg.Name)
		}
	}

	return properties, required
}

// inferResourceType determines the resource_type for a GraphQL field
// using longest-prefix matching against the configured resource prefixes.
func inferResourceType(fieldName string, prefixes map[string]string) string {
	if len(prefixes) == 0 {
		return ""
	}

	bestMatch := ""
	bestType := ""
	for prefix, resourceType := range prefixes {
		if strings.HasPrefix(fieldName, prefix) && len(prefix) > len(bestMatch) {
			bestMatch = prefix
			bestType = resourceType
		}
	}
	return bestType
}

// responseSchemaKey converts a GraphQL type name to a schema key.
// e.g. "IssuePayload" → "issue_payload", "Issue" → "issue"
func responseSchemaKey(typeName string) string {
	if typeName == "" {
		return ""
	}
	return toSnakeCase(typeName)
}

// buildSDLSchemas generates response schemas from SDL object types
// for types referenced by the generated actions.
func buildSDLSchemas(actions map[string]ActionDef, objectTypes map[string][]sdlField) map[string]SchemaDefinition {
	// Collect all referenced schema keys.
	referenced := make(map[string]bool)
	for _, action := range actions {
		if action.ResponseSchema != "" {
			referenced[action.ResponseSchema] = true
		}
	}

	if len(referenced) == 0 {
		return nil
	}

	schemas := make(map[string]SchemaDefinition)

	// For each referenced schema, find the matching object type and convert.
	for _, fields := range objectTypes {
		// Object types have been parsed; we need to match by schema key.
		// We'll iterate all object types and check if their key is referenced.
		_ = fields
	}

	// Direct approach: iterate all object types and include those whose
	// snake_case key is referenced, plus types they reference.
	secondaryRefs := make(map[string]bool)

	for typeName, fields := range objectTypes {
		schemaKey := toSnakeCase(typeName)
		if !referenced[schemaKey] {
			continue
		}

		schemaDef := convertObjectTypeToSchema(typeName, fields, objectTypes)
		schemas[schemaKey] = schemaDef

		// Collect secondary references (types used in this schema).
		for _, prop := range schemaDef.Properties {
			if prop.SchemaRef != "" {
				secondaryRefs[prop.SchemaRef] = true
			}
		}
	}

	// Include secondary referenced types.
	for typeName, fields := range objectTypes {
		schemaKey := toSnakeCase(typeName)
		if !secondaryRefs[schemaKey] || schemas[schemaKey].Type != "" {
			continue
		}
		schemas[schemaKey] = convertObjectTypeToSchema(typeName, fields, objectTypes)
	}

	if len(schemas) == 0 {
		return nil
	}
	return schemas
}

// buildIntrospectionSchemas generates response schemas from introspected types.
func buildIntrospectionSchemas(actions map[string]ActionDef, typeMap map[string]*IntrospectType) map[string]SchemaDefinition {
	referenced := make(map[string]bool)
	for _, action := range actions {
		if action.ResponseSchema != "" {
			referenced[action.ResponseSchema] = true
		}
	}
	if len(referenced) == 0 {
		return nil
	}

	schemas := make(map[string]SchemaDefinition)
	secondaryRefs := make(map[string]bool)

	for typeName, introType := range typeMap {
		if introType.Kind != "OBJECT" {
			continue
		}
		schemaKey := toSnakeCase(typeName)
		if !referenced[schemaKey] {
			continue
		}

		schemaDef := convertIntrospectTypeToSchema(introType, typeMap)
		schemas[schemaKey] = schemaDef

		for _, prop := range schemaDef.Properties {
			if prop.SchemaRef != "" {
				secondaryRefs[prop.SchemaRef] = true
			}
		}
	}

	for typeName, introType := range typeMap {
		if introType.Kind != "OBJECT" {
			continue
		}
		schemaKey := toSnakeCase(typeName)
		if !secondaryRefs[schemaKey] || schemas[schemaKey].Type != "" {
			continue
		}
		schemas[schemaKey] = convertIntrospectTypeToSchema(introType, typeMap)
	}

	if len(schemas) == 0 {
		return nil
	}
	return schemas
}

// convertObjectTypeToSchema converts SDL object type fields to a SchemaDefinition.
func convertObjectTypeToSchema(_ string, fields []sdlField, allObjectTypes map[string][]sdlField) SchemaDefinition {
	props := make(map[string]SchemaPropertyDef)

	for _, field := range fields {
		// Skip connection fields — they're pagination wrappers, not useful in flat schemas.
		returnBase := baseTypeName(field.Type)
		if isConnectionType(returnBase) {
			continue
		}
		// Skip fields with arguments (they're sub-queries, not simple properties).
		if len(field.Args) > 0 {
			continue
		}

		jsonType := sdlTypeToJSONSchema(field.Type)
		nullable := isNullableSDL(field.Type)

		prop := SchemaPropertyDef{
			Type:        jsonType,
			Description: truncateDescription(field.Description, 200),
		}
		if nullable {
			prop.Nullable = true
		}

		// If the field type is a known object type, add a schema_ref.
		if _, isObject := allObjectTypes[returnBase]; isObject && jsonType == "string" {
			prop.Type = "object"
			prop.SchemaRef = toSnakeCase(returnBase)
		}

		props[field.Name] = prop
	}

	return SchemaDefinition{
		Type:       "object",
		Properties: props,
	}
}

// convertIntrospectTypeToSchema converts an introspected type to a SchemaDefinition.
func convertIntrospectTypeToSchema(introType *IntrospectType, typeMap map[string]*IntrospectType) SchemaDefinition {
	props := make(map[string]SchemaPropertyDef)

	for _, field := range introType.Fields {
		returnTypeName := getNamedType(field.Type)

		// Skip connection types.
		if isConnectionType(returnTypeName) {
			continue
		}
		// Skip fields with arguments.
		if len(field.Args) > 0 {
			continue
		}

		jsonType := graphqlTypeToJSONSchema(field.Type)
		nullable := !isNonNull(field.Type)

		prop := SchemaPropertyDef{
			Type:        jsonType,
			Description: truncateDescription(field.Description, 200),
		}
		if nullable {
			prop.Nullable = true
		}

		// If the field type is a known OBJECT, add a schema_ref.
		if ref, ok := typeMap[returnTypeName]; ok && ref.Kind == "OBJECT" && jsonType == "string" {
			prop.Type = "object"
			prop.SchemaRef = toSnakeCase(returnTypeName)
		}

		props[field.Name] = prop
	}

	return SchemaDefinition{
		Type:       "object",
		Properties: props,
	}
}
