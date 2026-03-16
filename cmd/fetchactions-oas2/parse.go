package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pb33f/libopenapi"
	v2high "github.com/pb33f/libopenapi/datamodel/high/v2"
)

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
	Method       string            `json:"method"`
	Path         string            `json:"path"`
	BodyMapping  map[string]string `json:"body_mapping,omitempty"`
	QueryMapping map[string]string `json:"query_mapping,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	ResponsePath string            `json:"response_path,omitempty"`
}

// jsonSchemaProperty represents a single property in the JSON Schema.
type jsonSchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// parseSpec parses a Swagger 2.0 spec and returns a map of action key → ActionDef.
func parseSpec(specData []byte, cfg ServiceConfig) (map[string]ActionDef, error) {
	doc, err := libopenapi.NewDocument(specData)
	if err != nil {
		return nil, fmt.Errorf("parsing Swagger document: %w", err)
	}

	model, errs := doc.BuildV2Model()
	if model == nil {
		return nil, fmt.Errorf("building v2 model: %v", errs)
	}

	basePath := ""
	if model.Model.BasePath != "" {
		basePath = strings.TrimRight(model.Model.BasePath, "/")
	}

	actions := make(map[string]ActionDef)

	if model.Model.Paths == nil || model.Model.Paths.PathItems == nil {
		return actions, nil
	}

	for pair := model.Model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		rawPath := pair.Key()
		pathItem := pair.Value()

		fullPath := basePath + rawPath

		if !matchesFilters(fullPath, cfg.PathFilters, cfg.PathExcludes) {
			continue
		}

		ops := getV2Operations(pathItem)
		for method, op := range ops {
			if op.Deprecated {
				continue
			}

			actionKey, displayName := deriveV2ActionKey(op, method, fullPath)
			if actionKey == "" {
				continue
			}

			if _, exists := actions[actionKey]; exists {
				continue
			}

			desc := ""
			if op.Description != "" {
				desc = truncateDescription(op.Description, 200)
			} else if op.Summary != "" {
				desc = truncateDescription(op.Summary, 200)
			}

			resourceType := inferV2ResourceType(op, cfg.TagResourceMap)

			params, exec := buildV2ParamsAndExecution(op, pathItem.Parameters, method, fullPath, cfg.ExtraHeaders)

			access := inferAccess(method, actionKey, fullPath)

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

// getV2Operations returns method → operation for a Swagger 2.0 path item.
func getV2Operations(pi *v2high.PathItem) map[string]*v2high.Operation {
	ops := make(map[string]*v2high.Operation)
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

// buildV2ParamsAndExecution extracts Swagger 2.0 parameters and builds execution config.
func buildV2ParamsAndExecution(op *v2high.Operation, pathItemParams []*v2high.Parameter, method, path string, extraHeaders map[string]string) (json.RawMessage, *ExecutionConfig) {
	properties := make(map[string]jsonSchemaProperty)
	var required []string
	queryMapping := make(map[string]string)
	bodyMapping := make(map[string]string)

	processParam := func(param *v2high.Parameter) {
		name := param.Name
		if name == "" {
			return
		}
		// Skip formData (file uploads).
		if param.In == "formData" {
			return
		}
		if _, exists := properties[name]; exists {
			return
		}

		switch param.In {
		case "body":
			// Body param in Swagger 2.0: extract top-level properties from schema.
			if param.Schema != nil {
				s := param.Schema.Schema()
				if s != nil && s.Properties != nil {
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
					// Check required from body schema.
					for _, r := range s.Required {
						if _, exists := properties[r]; exists {
							required = append(required, r)
						}
					}
				}
			}
		case "path", "query":
			paramType := "string"
			if param.Type != "" {
				paramType = param.Type
			}
			desc := param.Description
			if len(desc) > 200 {
				desc = desc[:197] + "..."
			}
			properties[name] = jsonSchemaProperty{
				Type:        paramType,
				Description: desc,
			}
			if param.In == "path" {
				required = append(required, name)
			} else {
				queryMapping[name] = name
			}
		}
	}

	// Operation-level params first, then path-item-level.
	for _, param := range op.Parameters {
		processParam(param)
	}
	for _, param := range pathItemParams {
		processParam(param)
	}

	// Fallback: extract path template params.
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

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = dedupStrings(required)
	}
	paramsJSON, _ := json.Marshal(schema)

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

	return paramsJSON, exec
}

// dedupStrings removes duplicates from a string slice.
// readHintPrefixes indicate a read even when HTTP method is POST.
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
