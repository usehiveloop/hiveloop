package main

import "strings"

var readHintPrefixes = []string{
	"list", "get", "search", "find", "query", "read", "fetch", "check",
	"show", "describe", "lookup", "retrieve", "export", "download", "view",
}

var readHintPathSuffixes = []string{
	"/search", "/query", "/filter", "/find", "/list", "/lookup", "/export",
}

// inferAccess determines "read" or "write" from method, action key, and path.
func inferAccess(method, actionKey, path string) string {
	switch method {
	case "GET":
		return "read"
	case "POST":
		keyLower := strings.ToLower(actionKey)
		for _, prefix := range readHintPrefixes {
			if strings.Contains(keyLower, prefix) {
				return "read"
			}
		}
		pathLower := strings.ToLower(path)
		for _, suffix := range readHintPathSuffixes {
			if strings.HasSuffix(pathLower, suffix) {
				return "read"
			}
		}
		return "write"
	default:
		return "write"
	}
}
