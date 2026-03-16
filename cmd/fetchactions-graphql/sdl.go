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
}

// sdlField represents a single field on a Query or Mutation type.
type sdlField struct {
	Name        string
	Description string
	Args        []sdlArg
}

// sdlArg represents an argument on a field.
type sdlArg struct {
	Name        string
	Type        string // raw type string like "String!", "[ID!]!", "IssueCreateInput!"
	Description string
	Required    bool
}

// parseSDL parses a GraphQL SDL string and extracts Query/Mutation fields.
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
	queryFields := extractTypeFields(sdl, queryTypeName)
	schema.QueryFields = queryFields

	// Parse the Mutation type.
	mutationFields := extractTypeFields(sdl, mutationTypeName)
	schema.MutationFields = mutationFields

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
		// Try multiline args — if line has ( but no ), it spans multiple lines.
		// For now, handle the simple case only.
		return nil
	}

	field := &sdlField{
		Name:        m[1],
		Description: strings.TrimSpace(description),
	}

	// Parse args if present.
	if m[2] != "" {
		field.Args = parseArgs(m[2])
	}

	return field
}

// parseArgs parses a comma-separated argument list from inside parentheses.
func parseArgs(argsStr string) []sdlArg {
	var args []sdlArg

	// Split on commas, but be careful about nested types.
	parts := splitArgs(argsStr)
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
	t := strings.TrimSuffix(gqlType, "!")

	// Check for list.
	if strings.HasPrefix(t, "[") {
		return "array"
	}

	switch t {
	case "String", "ID", "DateTime", "Date", "URI", "URL", "TimelessDate", "JSONObject", "JSON":
		return "string"
	case "Int":
		return "integer"
	case "Float":
		return "number"
	case "Boolean":
		return "boolean"
	default:
		// Input objects, enums, custom scalars.
		return "string"
	}
}
