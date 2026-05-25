package main

import (
	"strings"

	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"
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

// matchesTags checks if an operation has at least one of the required tags.
func matchesTags(op *v3high.Operation, tagFilters []string) bool {
	if len(tagFilters) == 0 {
		return true
	}
	for _, opTag := range op.Tags {
		for _, filter := range tagFilters {
			if strings.EqualFold(opTag, filter) {
				return true
			}
		}
	}
	return false
}

// isFileUpload checks if the operation accepts multipart/form-data or octet-stream.
func isFileUpload(op *v3high.Operation) bool {
	if op.RequestBody == nil {
		return false
	}
	rb := op.RequestBody
	if rb.Content == nil {
		return false
	}
	for pair := rb.Content.First(); pair != nil; pair = pair.Next() {
		ct := pair.Key()
		if ct == "multipart/form-data" || ct == "application/octet-stream" {
			return true
		}
	}
	return false
}

// deriveActionKey produces the snake_case action key and display name.
func deriveActionKey(op *v3high.Operation, method, path string) (string, string) {
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

// inferResourceType uses tags + path patterns to determine the resource type.
func inferResourceType(op *v3high.Operation, path string, tagMap map[string]string) string {
	if tagMap != nil {
		for _, tag := range op.Tags {
			if rt, ok := tagMap[strings.ToLower(tag)]; ok {
				return rt
			}
			if rt, ok := tagMap[tag]; ok {
				return rt
			}
		}
	}
	return ""
}

// dedupStrings removes duplicates from a string slice.
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

// matchResourceByPath finds which resource a path belongs to.
// Exact paths are checked first, then longest prefix match wins.
func matchResourceByPath(path string, resources map[string]ResourceFilterConfig) string {
	for name, rc := range resources {
		for _, exactPath := range rc.ExactPaths {
			if path == exactPath {
				return name
			}
		}
	}

	bestName := ""
	bestLen := 0
	for name, rc := range resources {
		for _, prefix := range rc.PathPrefixes {
			if strings.HasPrefix(path, prefix) && len(prefix) > bestLen {
				bestName = name
				bestLen = len(prefix)
			}
		}
	}
	return bestName
}
