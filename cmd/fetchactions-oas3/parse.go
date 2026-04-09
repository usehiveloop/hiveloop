package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/pb33f/libopenapi"
	highbase "github.com/pb33f/libopenapi/datamodel/high/base"
	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"
)

var pathParamRe = regexp.MustCompile(`\{([^}]+)\}`)

// ActionDef mirrors the catalog ActionDef for JSON output.
type ActionDef struct {
	DisplayName    string           `json:"display_name"`
	Description    string           `json:"description"`
	Access         string           `json:"access"`
	ResourceType   string           `json:"resource_type"`
	Parameters     json.RawMessage  `json:"parameters"`
	Execution      *ExecutionConfig `json:"execution,omitempty"`
	ResponseSchema string           `json:"response_schema,omitempty"` // ref into top-level schemas map
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

// jsonSchemaProperty represents a single property in the JSON Schema.
type jsonSchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// SchemaProperty describes a single property in a flattened response schema.
type SchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Nullable    bool   `json:"nullable,omitempty"`
	SchemaRef   string `json:"schema_ref,omitempty"` // references another schema for nested object resolution
}

// FlatSchema is a flattened top-level-only representation of a response schema.
type FlatSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]SchemaProperty `json:"properties,omitempty"`
	Items      *FlatSchemaRef            `json:"items,omitempty"` // for array types
}

// FlatSchemaRef references another schema by name (for array item types).
type FlatSchemaRef struct {
	Ref string `json:"$ref,omitempty"`
}

// ParseResult holds parsed actions and the referenced response schemas.
type ParseResult struct {
	Actions map[string]ActionDef
	Schemas map[string]FlatSchema
}

// parseSpec parses an OpenAPI spec and returns actions + referenced response schemas.
func parseSpec(specData []byte, cfg ServiceConfig) (*ParseResult, error) {
	doc, err := libopenapi.NewDocument(specData)
	if err != nil {
		return nil, fmt.Errorf("parsing OpenAPI document: %w", err)
	}

	model, errs := doc.BuildV3Model()
	if model == nil {
		return nil, fmt.Errorf("building v3 model: %v", errs)
	}

	actions := make(map[string]ActionDef)
	schemas := make(map[string]FlatSchema)

	if model.Model.Paths == nil || model.Model.Paths.PathItems == nil {
		return &ParseResult{Actions: actions, Schemas: schemas}, nil
	}

	// Build components/schemas lookup for resolving $ref in response schemas.
	componentsMap := make(map[string]*highbase.SchemaProxy)
	if model.Model.Components != nil && model.Model.Components.Schemas != nil {
		for pair := model.Model.Components.Schemas.First(); pair != nil; pair = pair.Next() {
			componentsMap[pair.Key()] = pair.Value()
		}
	}

	useResources := len(cfg.Resources) > 0

	for pair := model.Model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
		path := pair.Key()
		pathItem := pair.Value()

		if useResources {
			// When resources are defined, skip paths that don't match any resource.
			if matchResourceByPath(path, cfg.Resources) == "" {
				continue
			}
		} else {
			if !matchesFilters(path, cfg.PathFilters, cfg.PathExcludes) {
				continue
			}
		}

		if cfg.BasePathStrip != "" {
			path = strings.TrimPrefix(path, cfg.BasePathStrip)
		}

		ops := getOperations(pathItem)
		for method, op := range ops {
			if op.Deprecated != nil && *op.Deprecated {
				continue
			}

			// Tag filtering only applies when not using resource-based filtering.
			if !useResources && !matchesTags(op, cfg.TagFilters) {
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

			var resourceType string
			if useResources {
				resourceType = matchResourceByPath(path, cfg.Resources)
			} else {
				resourceType = inferResourceType(op, path, cfg.TagResourceMap)
			}

			params, exec := buildParamsAndExecution(op, pathItem.Parameters, method, path, cfg.ExtraHeaders)

			access := inferAccess(method, actionKey, path)

			responseSchemaRef := extractV3ResponseSchema(op, componentsMap, schemas)

			actions[actionKey] = ActionDef{
				DisplayName:    displayName,
				Description:    desc,
				Access:         access,
				ResourceType:   resourceType,
				Parameters:     params,
				Execution:      exec,
				ResponseSchema: responseSchemaRef,
			}
		}
	}

	return &ParseResult{Actions: actions, Schemas: schemas}, nil
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

// extractV3ResponseSchema looks at the 200/201 response of an OpenAPI 3.x operation,
// resolves the schema ref, flattens it into the schemas map, and returns the ref name.
// Returns "" if no usable response schema is found.
func extractV3ResponseSchema(op *v3high.Operation, components map[string]*highbase.SchemaProxy, schemas map[string]FlatSchema) string {
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

		proxy := jsonContent.Schema
		schema := proxy.Schema()
		if schema == nil {
			continue
		}

		// Case 1: Direct $ref to a component schema.
		refName := extractV3RefName(proxy)
		if refName != "" {
			flattenV3Component(refName, components, schemas)
			return refName
		}

		// Case 2: Array with $ref items.
		if len(schema.Type) > 0 && schema.Type[0] == "array" && schema.Items != nil && schema.Items.A != nil {
			itemRef := extractV3RefName(schema.Items.A)
			if itemRef != "" {
				arraySchemaName := itemRef + "_list"
				if _, exists := schemas[arraySchemaName]; !exists {
					flattenV3Component(itemRef, components, schemas)
					schemas[arraySchemaName] = FlatSchema{
						Type:  "array",
						Items: &FlatSchemaRef{Ref: itemRef},
					}
				}
				return arraySchemaName
			}
		}

		// Case 3: Inline object — flatten it directly.
		if schema.Properties != nil {
			inlineName := ""
			if op.OperationId != "" {
				inlineName = toSnakeCase(op.OperationId) + "_response"
			} else {
				continue
			}
			if _, exists := schemas[inlineName]; !exists {
				flat := FlatSchema{
					Type:       "object",
					Properties: make(map[string]SchemaProperty),
				}
				for prop := schema.Properties.First(); prop != nil; prop = prop.Next() {
					flat.Properties[prop.Key()] = flattenV3Property(prop.Value())
				}
				schemas[inlineName] = flat
			}
			return inlineName
		}
	}

	return ""
}

// extractV3RefName pulls the component schema name from a SchemaProxy's $ref.
func extractV3RefName(proxy *highbase.SchemaProxy) string {
	if proxy == nil {
		return ""
	}
	ref := proxy.GetReference()
	if ref == "" {
		return ""
	}
	const prefix = "#/components/schemas/"
	if after, ok := strings.CutPrefix(ref, prefix); ok {
		return after
	}
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

// flattenV3Component resolves an OpenAPI 3.x component schema by name and stores
// its flattened top-level properties in the schemas map.
func flattenV3Component(name string, components map[string]*highbase.SchemaProxy, schemas map[string]FlatSchema) {
	if _, exists := schemas[name]; exists {
		return
	}

	proxy, ok := components[name]
	if !ok {
		return
	}
	schema := proxy.Schema()
	if schema == nil {
		return
	}

	flat := FlatSchema{
		Type:       "object",
		Properties: make(map[string]SchemaProperty),
	}

	if len(schema.Type) > 0 {
		flat.Type = schema.Type[0]
	}

	// For array types, try to resolve item ref.
	if flat.Type == "array" && schema.Items != nil && schema.Items.A != nil {
		itemRef := extractV3RefName(schema.Items.A)
		if itemRef != "" {
			flattenV3Component(itemRef, components, schemas)
			flat.Items = &FlatSchemaRef{Ref: itemRef}
		}
		schemas[name] = flat
		return
	}

	if schema.Properties != nil {
		for prop := schema.Properties.First(); prop != nil; prop = prop.Next() {
			flat.Properties[prop.Key()] = flattenV3Property(prop.Value())
		}
	}

	schemas[name] = flat

	// Transitively flatten any schemas referenced by properties via schema_ref.
	for _, propDef := range flat.Properties {
		if propDef.SchemaRef != "" {
			flattenV3Component(propDef.SchemaRef, components, schemas)
		}
	}
}

// flattenV3Property extracts type + description from a schema proxy without recursing.
// When the property is a $ref to a named component schema, the ref name is preserved
// as SchemaRef so the frontend can resolve nested object types.
func flattenV3Property(proxy *highbase.SchemaProxy) SchemaProperty {
	prop := SchemaProperty{Type: "string"}
	if proxy == nil {
		return prop
	}

	// Capture the $ref name before resolving the schema.
	refName := extractV3RefName(proxy)

	schema := proxy.Schema()
	if schema == nil {
		if refName != "" {
			prop.Type = "object"
			prop.SchemaRef = refName
		}
		return prop
	}

	if len(schema.Type) > 0 {
		prop.Type = schema.Type[0]
	} else if refName != "" {
		prop.Type = "object"
	}

	// Preserve the ref name for object types so the frontend can drill into nested schemas.
	if refName != "" && prop.Type == "object" {
		prop.SchemaRef = refName
	}

	if schema.Description != "" {
		prop.Description = schema.Description
	}

	if schema.Nullable != nil && *schema.Nullable {
		prop.Nullable = true
	}

	return prop
}

// matchResourceByPath finds which resource a path belongs to.
// Returns the resource name, or "" if no match.
// Exact paths are checked first, then longest prefix match wins.
func matchResourceByPath(path string, resources map[string]ResourceFilterConfig) string {
	// Check exact paths first (highest priority).
	for name, rc := range resources {
		for _, exactPath := range rc.ExactPaths {
			if path == exactPath {
				return name
			}
		}
	}

	// Find longest matching prefix.
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
