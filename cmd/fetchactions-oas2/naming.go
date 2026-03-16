package main

import (
	"regexp"
	"strings"
	"unicode"
)

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9]+`)
var pathParamRe = regexp.MustCompile(`\{([^}]+)\}`)

// toSnakeCase converts an operationId to a snake_case action key.
func toSnakeCase(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")

	var result []rune
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 {
			prev := runes[i-1]
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				result = append(result, '_')
			} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				result = append(result, '_')
			}
		}
		result = append(result, unicode.ToLower(r))
	}

	s = string(result)
	s = nonAlphaNum.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	return s
}

// toDisplayName converts a snake_case key to "Title Case Words".
func toDisplayName(snakeKey string) string {
	words := strings.Split(snakeKey, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// fallbackActionKey generates an action key from method + path.
func fallbackActionKey(method, path string) string {
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	key := strings.ToLower(method)
	for _, p := range parts {
		if p != "" {
			key += "_" + toSnakeCase(p)
		}
	}
	return key
}

// truncateDescription truncates to maxLen characters.
func truncateDescription(desc string, maxLen int) string {
	desc = strings.Join(strings.Fields(desc), " ")
	if len(desc) <= maxLen {
		return desc
	}
	return desc[:maxLen-3] + "..."
}
