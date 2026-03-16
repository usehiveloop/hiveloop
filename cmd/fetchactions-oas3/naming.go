package main

import (
	"regexp"
	"strings"
	"unicode"
)

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// toSnakeCase converts an operationId like "repos/list-for-authenticated-user"
// or "listIssues" to a snake_case action key like "list_repos_for_authenticated_user"
// or "list_issues".
func toSnakeCase(s string) string {
	// Replace common separators with underscores.
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")

	// Insert underscore before uppercase letters in camelCase.
	var result []rune
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 {
			prev := runes[i-1]
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				result = append(result, '_')
			} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				// Handle acronyms: "getHTTPResponse" → "get_http_response"
				result = append(result, '_')
			}
		}
		result = append(result, unicode.ToLower(r))
	}

	s = string(result)
	// Collapse multiple underscores and trim.
	s = nonAlphaNum.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")

	return s
}

// toDisplayName converts a snake_case action key to a human-readable display name.
// e.g. "list_issues" → "List Issues", "create_pull_request" → "Create Pull Request"
func toDisplayName(snakeKey string) string {
	words := strings.Split(snakeKey, "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// fallbackActionKey generates an action key from HTTP method + path when no operationId exists.
// e.g. GET /repos/{owner}/{repo}/issues → "get_repos_owner_repo_issues"
func fallbackActionKey(method, path string) string {
	// Remove path parameter braces.
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

// truncateDescription truncates a description to maxLen characters, adding "..." if truncated.
func truncateDescription(desc string, maxLen int) string {
	// Collapse newlines to spaces.
	desc = strings.Join(strings.Fields(desc), " ")
	if len(desc) <= maxLen {
		return desc
	}
	return desc[:maxLen-3] + "..."
}
