package main

import (
	"sort"
	"strings"
)

// buildSDLParams converts SDL arguments to JSON Schema properties,
// expanding input object types into nested object schemas.
func buildSDLParams(args []sdlArg, inputTypes map[string][]sdlField) (map[string]jsonSchemaProperty, []string) {
	properties := make(map[string]jsonSchemaProperty)
	var required []string

	for _, arg := range args {
		typeName := baseTypeName(arg.Type)

		if inputFields, ok := inputTypes[typeName]; ok {
			nested := make(map[string]jsonSchemaProperty)
			var nestedRequired []string

			for _, inputField := range inputFields {
				fieldTypeName := baseTypeName(inputField.Type)

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

		if inputFields, ok := inputTypeFields[typeName]; ok {
			nested := make(map[string]jsonSchemaProperty)
			var nestedRequired []string

			for _, inputField := range inputFields {
				innerTypeName := getNamedType(inputField.Type)

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
func responseSchemaKey(typeName string) string {
	if typeName == "" {
		return ""
	}
	return toSnakeCase(typeName)
}
