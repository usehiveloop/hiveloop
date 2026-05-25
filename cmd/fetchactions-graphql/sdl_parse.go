package main

import (
	"regexp"
)

// parseSDL parses a GraphQL SDL string and extracts Query/Mutation fields,
// input types, and object types.
func parseSDL(sdl string) (*sdlSchema, error) {
	schema := &sdlSchema{}

	queryTypeName := "Query"
	mutationTypeName := "Mutation"

	schemaBlockRe := regexp.MustCompile(`(?s)schema\s*\{([^}]+)\}`)
	if m := schemaBlockRe.FindStringSubmatch(sdl); m != nil {
		if qm := regexp.MustCompile(`query\s*:\s*(\w+)`).FindStringSubmatch(m[1]); qm != nil {
			queryTypeName = qm[1]
		}
		if mm := regexp.MustCompile(`mutation\s*:\s*(\w+)`).FindStringSubmatch(m[1]); mm != nil {
			mutationTypeName = mm[1]
		}
	}

	schema.QueryFields = extractTypeFields(sdl, queryTypeName)
	schema.MutationFields = extractTypeFields(sdl, mutationTypeName)
	schema.InputTypes = parseAllInputTypes(sdl)
	schema.ObjectTypes = parseAllObjectTypes(sdl)

	return schema, nil
}

// extractTypeFields finds a type block in SDL and extracts its fields.
func extractTypeFields(sdl, typeName string) []sdlField {
	typeStartRe := regexp.MustCompile(`(?m)^type\s+` + regexp.QuoteMeta(typeName) + `\b[^{]*\{`)
	loc := typeStartRe.FindStringIndex(sdl)
	if loc == nil {
		return nil
	}

	body := extractBraceBlock(sdl[loc[1]-1:])
	if body == "" {
		return nil
	}

	return parseFieldsFromBlock(body)
}

// parseAllInputTypes finds all `input Xxx { ... }` blocks and parses their fields.
func parseAllInputTypes(sdl string) map[string][]sdlField {
	result := make(map[string][]sdlField)
	inputStartRe := regexp.MustCompile(`(?m)^input\s+(\w+)\b[^{]*\{`)

	for _, match := range inputStartRe.FindAllStringSubmatchIndex(sdl, -1) {
		typeName := sdl[match[2]:match[3]]
		body := extractBraceBlock(sdl[match[0]:])
		if body == "" {
			continue
		}
		fields := parseFieldsFromBlock(body)
		result[typeName] = fields
	}
	return result
}

// parseAllObjectTypes finds all `type Xxx { ... }` blocks and parses their fields.
func parseAllObjectTypes(sdl string) map[string][]sdlField {
	result := make(map[string][]sdlField)
	typeStartRe := regexp.MustCompile(`(?m)^type\s+(\w+)\b[^{]*\{`)

	for _, match := range typeStartRe.FindAllStringSubmatchIndex(sdl, -1) {
		typeName := sdl[match[2]:match[3]]
		body := extractBraceBlock(sdl[match[0]:])
		if body == "" {
			continue
		}
		fields := parseFieldsFromBlock(body)
		result[typeName] = fields
	}
	return result
}

// extractBraceBlock extracts content between the first { and its matching }.
func extractBraceBlock(s string) string {
	depth := 0
	start := -1
	for i, ch := range s {
		switch ch {
		case '{':
			if depth == 0 {
				start = i + 1
			}
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start:i]
			}
		}
	}
	return ""
}
