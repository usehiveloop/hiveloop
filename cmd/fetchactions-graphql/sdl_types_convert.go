package main

import "strings"

// sdlTypeToJSONSchema maps a GraphQL SDL type string to a JSON Schema type.
func sdlTypeToJSONSchema(gqlType string) string {
	typeName := strings.TrimSuffix(gqlType, "!")

	if strings.HasPrefix(typeName, "[") {
		return "array"
	}

	switch typeName {
	case "String", "ID", "DateTime", "Date", "URI", "URL", "TimelessDate", "JSONObject", "JSON":
		return "string"
	case "Int":
		return "integer"
	case "Float":
		return "number"
	case "Boolean":
		return "boolean"
	default:
		return "string"
	}
}

// baseTypeName strips NonNull (!) and List ([]) wrappers to get the named type.
func baseTypeName(gqlType string) string {
	typeName := strings.TrimSuffix(gqlType, "!")
	typeName = strings.TrimPrefix(typeName, "[")
	typeName = strings.TrimSuffix(typeName, "]")
	typeName = strings.TrimSuffix(typeName, "!")
	return typeName
}

// isNullableSDL returns true if the SDL type is nullable (no trailing !).
func isNullableSDL(gqlType string) bool {
	return !strings.HasSuffix(gqlType, "!")
}

// isConnectionType returns true if the type name ends with "Connection".
func isConnectionType(typeName string) bool {
	return strings.HasSuffix(typeName, "Connection")
}
