package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const sdlCacheDir = "/tmp/fetchactions-cache"

// fetchSDL downloads a .graphql SDL file, caching it locally.
func fetchSDL(url string, force bool) (string, error) {
	if err := os.MkdirAll(sdlCacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache dir: %w", err)
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(url)))
	cachePath := filepath.Join(sdlCacheDir, "sdl-"+hash)

	if !force {
		if data, err := os.ReadFile(cachePath); err == nil {
			return string(data), nil
		}
	}

	fmt.Printf("  Downloading %s ...\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("fetching %s: status %d: %s", url, resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	_ = os.WriteFile(cachePath, data, 0644)
	return string(data), nil
}

// sdlSchema holds the parsed Query and Mutation fields from an SDL file.
type sdlSchema struct {
	QueryFields    []sdlField
	MutationFields []sdlField
	InputTypes     map[string][]sdlField // input type name → fields
	ObjectTypes    map[string][]sdlField // object type name → fields (for response schemas)
}

// sdlField represents a single field on a Query, Mutation, input, or object type.
type sdlField struct {
	Name        string
	Description string
	Args        []sdlArg
	Type        string // field's return/value type (e.g., "String!", "Issue!", "[Issue!]!")
}

// sdlArg represents an argument on a field.
type sdlArg struct {
	Name        string
	Type        string // raw type string like "String!", "[ID!]!", "IssueCreateInput!"
	Description string
	Required    bool
}

// parseSDL parses a GraphQL SDL string and extracts Query/Mutation fields,
// input types, and object types.
func parseSDL(sdl string) (*sdlSchema, error) {
	schema := &sdlSchema{}

	// Find all type definitions and resolve the Query/Mutation root type names.
	queryTypeName := "Query"
	mutationTypeName := "Mutation"

	// Check for schema { query: ..., mutation: ... } block.
	schemaBlockRe := regexp.MustCompile(`(?s)schema\s*\{([^}]+)\}`)
	if m := schemaBlockRe.FindStringSubmatch(sdl); m != nil {
		if qm := regexp.MustCompile(`query\s*:\s*(\w+)`).FindStringSubmatch(m[1]); qm != nil {
			queryTypeName = qm[1]
		}
		if mm := regexp.MustCompile(`mutation\s*:\s*(\w+)`).FindStringSubmatch(m[1]); mm != nil {
			mutationTypeName = mm[1]
		}
	}

	// Parse the Query type.
	schema.QueryFields = extractTypeFields(sdl, queryTypeName)

	// Parse the Mutation type.
	schema.MutationFields = extractTypeFields(sdl, mutationTypeName)

	// Parse all input types for parameter expansion.
	schema.InputTypes = parseAllInputTypes(sdl)

	// Parse all object types for response schemas.
	schema.ObjectTypes = parseAllObjectTypes(sdl)

	return schema, nil
}

// extractTypeFields finds a type block in SDL and extracts its fields.
func extractTypeFields(sdl, typeName string) []sdlField {
	// Match: type TypeName { ... } or type TypeName implements ... { ... }
	// We need to handle nested braces in descriptions, so find the type start
	// and manually match braces.
	typeStartRe := regexp.MustCompile(`(?m)^type\s+` + regexp.QuoteMeta(typeName) + `\b[^{]*\{`)
	loc := typeStartRe.FindStringIndex(sdl)
	if loc == nil {
		return nil
	}

	// Find matching closing brace.
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
		// Extract the brace block starting from the match position.
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
// Used for generating response schemas.
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

// parseFieldsFromBlock parses individual field definitions from a type body.
// Handles multiline field definitions where arguments span multiple lines.
func parseFieldsFromBlock(block string) []sdlField {
	var fields []sdlField
	lines := strings.Split(block, "\n")

	var currentDesc strings.Builder
	var fieldAccum strings.Builder
	inBlockComment := false
	parenDepth := 0

	flushField := func() {
		if fieldAccum.Len() == 0 {
			return
		}
		fieldStr := strings.TrimSpace(fieldAccum.String())
		fieldAccum.Reset()

		field := parseFieldLine(fieldStr, strings.TrimSpace(currentDesc.String()))
		currentDesc.Reset()
		if field != nil {
			fields = append(fields, *field)
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines (but not while accumulating a multiline field).
		if trimmed == "" && parenDepth == 0 {
			continue
		}

		// Handle block string descriptions: """..."""
		if strings.HasPrefix(trimmed, `"""`) && parenDepth == 0 {
			if inBlockComment {
				inBlockComment = false
				continue
			}
			if strings.Count(trimmed, `"""`) >= 2 {
				desc := strings.TrimPrefix(trimmed, `"""`)
				desc = strings.TrimSuffix(desc, `"""`)
				currentDesc.Reset()
				currentDesc.WriteString(strings.TrimSpace(desc))
				continue
			}
			inBlockComment = true
			currentDesc.Reset()
			continue
		}
		if inBlockComment {
			currentDesc.WriteString(trimmed)
			currentDesc.WriteString(" ")
			continue
		}

		// Handle single-line string descriptions.
		if strings.HasPrefix(trimmed, `"`) && !strings.HasPrefix(trimmed, `""`) && parenDepth == 0 {
			desc := strings.Trim(trimmed, `"`)
			currentDesc.Reset()
			currentDesc.WriteString(desc)
			continue
		}

		// Handle # comments — skip.
		if strings.HasPrefix(trimmed, "#") && parenDepth == 0 {
			continue
		}

		// Accumulate field lines, tracking parenthesis depth.
		if fieldAccum.Len() > 0 {
			fieldAccum.WriteString(" ")
		}
		fieldAccum.WriteString(trimmed)

		for _, ch := range trimmed {
			switch ch {
			case '(':
				parenDepth++
			case ')':
				parenDepth--
			}
		}

		// If parens are balanced, the field definition is complete.
		if parenDepth <= 0 {
			parenDepth = 0
			flushField()
		}
	}

	// Flush any remaining accumulated field.
	flushField()

	return fields
}

// fieldLineRe matches: fieldName(args...): ReturnType
var fieldLineRe = regexp.MustCompile(`^(\w+)\s*(?:\(([^)]*)\))?\s*:\s*(.+?)(?:\s*@.*)?$`)

// parseFieldLine parses a single field definition line.
func parseFieldLine(line, description string) *sdlField {
	// Remove trailing directives and clean up.
	line = strings.TrimSpace(line)

	m := fieldLineRe.FindStringSubmatch(line)
	if m == nil {
		return nil
	}

	field := &sdlField{
		Name:        m[1],
		Description: strings.TrimSpace(description),
		Type:        strings.TrimSpace(m[3]),
	}

	// Parse args if present.
	if m[2] != "" {
		field.Args = parseArgs(m[2])
	}

	return field
}

// parseArgs parses an argument list from inside parentheses.
// Handles both comma-separated args and whitespace-separated args (common in SDL
// files where args appear on separate lines without commas).
func parseArgs(argsStr string) []sdlArg {
	argsStr = strings.TrimSpace(argsStr)
	if argsStr == "" {
		return nil
	}

	// Try comma-separated first (most common).
	if strings.Contains(argsStr, ",") {
		parts := splitArgs(argsStr)
		var args []sdlArg
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			arg := parseOneArg(part)
			if arg != nil {
				args = append(args, *arg)
			}
		}
		if len(args) > 0 {
			return args
		}
	}

	// Fallback: extract all name:Type patterns with optional """desc""" prefixes.
	// This handles SDL where multiline args are joined into a single string
	// without comma delimiters (e.g. `""" desc """ id: String! """ desc """ input: FooInput!`).
	return parseArgsRegex(argsStr)
}

// sdlArgFinder matches individual arguments in a joined arg string.
// Captures: (1) description inside """, (2) arg name, (3) type including list/non-null wrappers.
var sdlArgFinder = regexp.MustCompile(`(?:"""((?:[^"]|"[^"]|""[^"])*)"""\s+)?(\w+)\s*:\s*(\[?\w+!?\]?!?)`)

// parseArgsRegex extracts all arguments from a string using regex matching.
// Used when args lack comma delimiters.
func parseArgsRegex(argsStr string) []sdlArg {
	matches := sdlArgFinder.FindAllStringSubmatch(argsStr, -1)
	var args []sdlArg

	for _, match := range matches {
		desc := strings.TrimSpace(match[1])
		// Collapse multi-line descriptions into single line.
		desc = strings.Join(strings.Fields(desc), " ")
		name := match[2]
		typeStr := match[3]
		required := strings.HasSuffix(typeStr, "!")

		args = append(args, sdlArg{
			Name:        name,
			Type:        typeStr,
			Description: desc,
			Required:    required,
		})
	}

	return args
}

// splitArgs splits an args string on commas, respecting nested brackets.
func splitArgs(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, ch := range s {
		switch ch {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// argRe matches: argName: Type or argName: Type = defaultValue
var argRe = regexp.MustCompile(`^(?:"""[^"]*"""\s*)?(\w+)\s*:\s*([^\s=]+(?:\s*=\s*.+)?)$`)

// parseOneArg parses a single argument definition.
func parseOneArg(s string) *sdlArg {
	s = strings.TrimSpace(s)

	// Strip inline description.
	desc := ""
	if strings.HasPrefix(s, `"""`) {
		end := strings.Index(s[3:], `"""`)
		if end >= 0 {
			desc = strings.TrimSpace(s[3 : 3+end])
			s = strings.TrimSpace(s[3+end+3:])
		}
	} else if strings.HasPrefix(s, `"`) {
		end := strings.Index(s[1:], `"`)
		if end >= 0 {
			desc = strings.TrimSpace(s[1 : 1+end])
			s = strings.TrimSpace(s[1+end+1:])
		}
	}

	m := argRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}

	name := m[1]
	typeStr := m[2]

	// Strip default value.
	if idx := strings.Index(typeStr, "="); idx >= 0 {
		typeStr = strings.TrimSpace(typeStr[:idx])
	}

	required := strings.HasSuffix(typeStr, "!")

	return &sdlArg{
		Name:        name,
		Type:        typeStr,
		Description: desc,
		Required:    required,
	}
}

// sdlTypeToJSONSchema maps a GraphQL SDL type string to a JSON Schema type.
func sdlTypeToJSONSchema(gqlType string) string {
	// Strip non-null marker.
	typeName := strings.TrimSuffix(gqlType, "!")

	// Check for list.
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
		// Enums, custom scalars — treat as string.
		return "string"
	}
}

// baseTypeName strips NonNull (!) and List ([]) wrappers to get the named type.
// e.g. "IssueCreateInput!" → "IssueCreateInput", "[String!]!" → "String"
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
