package main

// buildSDLSchemas generates response schemas from SDL object types
// for types referenced by the generated actions.
func buildSDLSchemas(actions map[string]ActionDef, objectTypes map[string][]sdlField) map[string]SchemaDefinition {
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

	for typeName, fields := range objectTypes {
		schemaKey := toSnakeCase(typeName)
		if !referenced[schemaKey] {
			continue
		}

		schemaDef := convertObjectTypeToSchema(typeName, fields, objectTypes)
		schemas[schemaKey] = schemaDef

		for _, prop := range schemaDef.Properties {
			if prop.SchemaRef != "" {
				secondaryRefs[prop.SchemaRef] = true
			}
		}
	}

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
		returnBase := baseTypeName(field.Type)
		if isConnectionType(returnBase) {
			continue
		}
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

		if isConnectionType(returnTypeName) {
			continue
		}
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
