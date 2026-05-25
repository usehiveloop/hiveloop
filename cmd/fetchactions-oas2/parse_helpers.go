package main

import (
	"strings"

	v2high "github.com/pb33f/libopenapi/datamodel/high/v2"
)

// matchesFilters checks if a path matches include/exclude filters.
func matchesFilters(path string, includes, excludes []string) bool {
	if len(includes) > 0 {
		matched := false
		for _, prefix := range includes {
			if strings.HasPrefix(path, prefix) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, prefix := range excludes {
		if strings.HasPrefix(path, prefix) {
			return false
		}
	}
	return true
}

// deriveV2ActionKey produces the action key and display name from a Swagger operation.
func deriveV2ActionKey(op *v2high.Operation, method, path string) (string, string) {
	var key string
	if op.OperationId != "" {
		key = toSnakeCase(op.OperationId)
	} else {
		key = fallbackActionKey(method, path)
	}

	displayName := ""
	if op.Summary != "" {
		displayName = op.Summary
	} else {
		displayName = toDisplayName(key)
	}
	if len(displayName) > 80 {
		displayName = displayName[:77] + "..."
	}
	return key, displayName
}

// inferV2ResourceType uses tags to determine resource type.
func inferV2ResourceType(op *v2high.Operation, tagMap map[string]string) string {
	if tagMap == nil {
		return ""
	}
	for _, tag := range op.Tags {
		if rt, ok := tagMap[strings.ToLower(tag)]; ok {
			return rt
		}
		if rt, ok := tagMap[tag]; ok {
			return rt
		}
	}
	return ""
}

func dedupStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
