package main

import (
	"strings"
)

var readHintVerbs = map[string]bool{
	"list": true, "get": true, "search": true, "find": true, "query": true,
	"read": true, "fetch": true, "check": true, "show": true, "describe": true,
	"lookup": true, "retrieve": true, "export": true, "download": true, "view": true,
}

var readHintPathSuffixes = []string{
	"/search", "/query", "/filter", "/find", "/list", "/lookup", "/export",
}

// inferAccess determines whether an action is "read" or "write" based on
// HTTP method, action key, and path. GET is always read. POST is read if
// the action key's verb is a read verb, or if the path ends in a search-like suffix.
// PUT, PATCH, DELETE are always write.
func inferAccess(method, actionKey, path string) string {
	switch method {
	case "GET":
		return "read"
	case "POST":
		tokens := strings.Split(strings.ToLower(actionKey), "_")
		for index := 0; index < len(tokens) && index < 2; index++ {
			if readHintVerbs[tokens[index]] {
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
