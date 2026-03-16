package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/pb33f/libopenapi"
	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"
)

var pathParamRe = regexp.MustCompile(`\{([^}]+)\}`)

// ActionDef mirrors the catalog ActionDef for JSON output.
type ActionDef struct {
	DisplayName  string           `json:"display_name"`
	Description  string           `json:"description"`
	Access       string           `json:"access"`
	ResourceType string           `json:"resource_type"`
	Parameters   json.RawMessage  `json:"parameters"`
	Execution    *ExecutionConfig `json:"execution,omitempty"`
}

// ExecutionConfig mirrors the catalog ExecutionConfig.
type ExecutionConfig struct {
	Method       string            `json:"method"`                  // HTTP method
	Path         string            `json:"path"`                    // API path with {param} placeholders
	BodyMapping  map[string]string `json:"body_mapping,omitempty"`  // param → body field
	QueryMapping map[string]string `json:"query_mapping,omitempty"` // param → query param
	Headers      map[string]string `json:"headers,omitempty"`       // extra headers
	ResponsePath string            `json:"response_path,omitempty"` // dot-path to extract response data
}

// parseSpec parses an OpenAPI spec and returns a map of action key → ActionDef.
func parseSpec(specData []byte, cfg ServiceConfig) (map[string]ActionDef, error) {
	doc, err := libopenapi.NewDocument(specData)
	if err != nil {
		return nil, fmt.Errorf("parsing OpenAPI document: %w", err)
	}

	model, errs := doc.BuildV3Model()
	if model == nil {
		return nil, fmt.Errorf("building v3 model: %v", errs)
	}

	actions := make(map[string]ActionDef)

	if model.Model.Paths == nil || model.Model.Paths.PathItems == nil {
		return actions, nil
	}

	for pair := model.Model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		path := pair.Key()
		pathItem := pair.Value()

		if !matchesFilters(path, cfg.PathFilters, cfg.PathExcludes) {
			continue
		}

		if cfg.BasePathStrip != "" {
			path = strings.TrimPrefix(path, cfg.BasePathStrip)
		}

		ops := getOperations(pathItem)
		for method, op := range ops {
			if op.Deprecated != nil && *op.Deprecated {
				continue
			}

			if !matchesTags(op, cfg.TagFilters) {
				continue
			}

			// Skip file upload endpoints.
			if isFileUpload(op) {
				continue
			}

			actionKey, displayName := deriveActionKey(op, method, path)
			if actionKey == "" {
				continue
			}

			// Deduplicate: keep first occurrence.
			if _, exists := actions[actionKey]; exists {
				continue
			}

			desc := ""
			if op.Description != "" {
				desc = truncateDescription(op.Description, 200)
			} else if op.Summary != "" {
				desc = truncateDescription(op.Summary, 200)
			}

			resourceType := inferResourceType(op, path, cfg.TagResourceMap)

			params, exec := buildParamsAndExecution(op, pathItem.Parameters, method, path, cfg.ExtraHeaders)

			access := inferAccess(method, actionKey, path)

			actions[actionKey] = ActionDef{
				DisplayName:  displayName,
				Description:  desc,
				Access:       access,
				ResourceType: resourceType,
				Parameters:   params,
				Execution:    exec,
			}
		}
	}

	return actions, nil
}

// getOperations returns a map of HTTP method → operation for a path item.
func getOperations(pi *v3high.PathItem) map[string]*v3high.Operation {
	ops := make(map[string]*v3high.Operation)
	if pi.Get != nil {
		ops["GET"] = pi.Get
	}
	if pi.Post != nil {
		ops["POST"] = pi.Post
	}
	if pi.Put != nil {
		ops["PUT"] = pi.Put
	}
	if pi.Patch != nil {
		ops["PATCH"] = pi.Patch
	}
	if pi.Delete != nil {
		ops["DELETE"] = pi.Delete
	}
	return ops
}

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

	// Truncate display name.
	if len(displayName) > 80 {
		displayName = displayName[:77] + "..."
	}

	return key, displayName
}

// inferResourceType uses tags + path patterns to determine the resource type.
func inferResourceType(op *v3high.Operation, path string, tagMap map[string]string) string {
	// Try tag-based mapping first.
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
	// No resource type inference from path alone — return empty.
	return ""
}

// jsonSchemaProperty represents a single property in the JSON Schema.
type jsonSchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// buildParamsAndExecution extracts parameters and builds execution config.
// pathItemParams are parameters defined at the path-item level (shared across all operations).
func buildParamsAndExecution(op *v3high.Operation, pathItemParams []*v3high.Parameter, method, path string, extraHeaders map[string]string) (json.RawMessage, *ExecutionConfig) {
	properties := make(map[string]jsonSchemaProperty)
	var required []string
	queryMapping := make(map[string]string)
	bodyMapping := make(map[string]string)

	// processParam handles a single parameter (from either path-item or operation level).
	processParam := func(param *v3high.Parameter) {
		name := param.Name
		if name == "" {
			return
		}
		// Skip if already defined (operation-level overrides path-item-level).
		if _, exists := properties[name]; exists {
			return
		}

		paramType := "string"
		if param.Schema != nil {
			s := param.Schema.Schema()
			if s != nil && len(s.Type) > 0 {
				paramType = s.Type[0]
			}
		}

		desc := param.Description
		if len(desc) > 200 {
			desc = desc[:197] + "..."
		}

		properties[name] = jsonSchemaProperty{
			Type:        paramType,
			Description: desc,
		}

		switch param.In {
		case "path":
			required = append(required, name)
		case "query":
			queryMapping[name] = name
		}
	}

	// Process operation-level parameters first (they take precedence).
	for _, param := range op.Parameters {
		processParam(param)
	}
	// Then path-item-level parameters.
	for _, param := range pathItemParams {
		processParam(param)
	}

	// Fallback: extract path template params that weren't found in parameter objects.
	for _, match := range pathParamRe.FindAllStringSubmatch(path, -1) {
		paramName := match[1]
		if _, exists := properties[paramName]; !exists {
			properties[paramName] = jsonSchemaProperty{
				Type:        "string",
				Description: paramName,
			}
			required = append(required, paramName)
		}
	}

	// Process request body (top-level properties only).
	if op.RequestBody != nil && op.RequestBody.Content != nil {
		for pair := op.RequestBody.Content.First(); pair != nil; pair = pair.Next() {
			ct := pair.Key()
			if ct != "application/json" {
				continue
			}
			mt := pair.Value()
			if mt.Schema == nil {
				continue
			}
			s := mt.Schema.Schema()
			if s == nil || s.Properties == nil {
				continue
			}

			for prop := s.Properties.First(); prop != nil; prop = prop.Next() {
				propName := prop.Key()
				propSchema := prop.Value()

				if _, exists := properties[propName]; exists {
					continue
				}

				propType := "string"
				propDesc := ""
				if propSchema != nil {
					ps := propSchema.Schema()
					if ps != nil {
						if len(ps.Type) > 0 {
							propType = ps.Type[0]
						}
						propDesc = ps.Description
						if len(propDesc) > 200 {
							propDesc = propDesc[:197] + "..."
						}
					}
				}

				properties[propName] = jsonSchemaProperty{
					Type:        propType,
					Description: propDesc,
				}
				bodyMapping[propName] = propName
			}

			// Check required fields from body schema.
			if len(s.Required) > 0 {
				for _, r := range s.Required {
					if _, exists := properties[r]; exists {
						required = append(required, r)
					}
				}
			}
			break // Only process the first application/json content type.
		}
	}

	// Build JSON Schema for parameters.
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = dedupStrings(required)
	}
	paramsJSON, _ := json.Marshal(schema)

	// Build execution config.
	exec := &ExecutionConfig{
		Method: method,
		Path:   path,
	}
	if len(queryMapping) > 0 {
		exec.QueryMapping = queryMapping
	}
	if len(bodyMapping) > 0 {
		exec.BodyMapping = bodyMapping
	}
	if len(extraHeaders) > 0 {
		exec.Headers = extraHeaders
	}

	// Try to infer response_path from 200 response schema.
	exec.ResponsePath = inferResponsePath(op)

	return paramsJSON, exec
}

// inferResponsePath looks at the 200/201 response schema for array types or known wrapper patterns.
func inferResponsePath(op *v3high.Operation) string {
	if op.Responses == nil || op.Responses.Codes == nil {
		return ""
	}

	for _, code := range []string{"200", "201"} {
		resp := op.Responses.Codes.GetOrZero(code)
		if resp == nil || resp.Content == nil {
			continue
		}
		jsonContent := resp.Content.GetOrZero("application/json")
		if jsonContent == nil || jsonContent.Schema == nil {
			continue
		}
		s := jsonContent.Schema.Schema()
		if s == nil {
			continue
		}

		// If the response itself is an array, no path needed.
		if len(s.Type) > 0 && s.Type[0] == "array" {
			return ""
		}

		// Look for common wrapper patterns: {items: [...], values: [...], data: [...], results: [...]}
		if s.Properties != nil {
			for _, wrapperKey := range []string{"items", "values", "data", "results", "records", "entries", "nodes", "objects"} {
				prop := s.Properties.GetOrZero(wrapperKey)
				if prop == nil {
					continue
				}
				ps := prop.Schema()
				if ps != nil && len(ps.Type) > 0 && ps.Type[0] == "array" {
					return wrapperKey
				}
			}
		}
	}
	return ""
}

// readHintPrefixes are operationId/action-key substrings that indicate a read operation
// even when the HTTP method is POST.
var readHintPrefixes = []string{
	"list", "get", "search", "find", "query", "read", "fetch", "check",
	"show", "describe", "lookup", "retrieve", "export", "download", "view",
}

// readHintPathSuffixes are path segments that indicate a read operation via POST.
var readHintPathSuffixes = []string{
	"/search", "/query", "/filter", "/find", "/list", "/lookup", "/export",
}

// inferAccess determines whether an action is "read" or "write" based on
// HTTP method, action key, and path. GET is always read. POST is read if
// the action key or path contains read-like terms (search, query, list, etc.).
// PUT, PATCH, DELETE are always write.
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
	default: // PUT, PATCH, DELETE
		return "write"
	}
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
