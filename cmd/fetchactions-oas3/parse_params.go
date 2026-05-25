package main

import (
	"encoding/json"

	highbase "github.com/pb33f/libopenapi/datamodel/high/base"
	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// buildParamsAndExecution extracts parameters and builds execution config.
func buildParamsAndExecution(op *v3high.Operation, pathItemParams []*v3high.Parameter, method, path string, extraHeaders map[string]string) (json.RawMessage, *ExecutionConfig) {
	properties := make(map[string]jsonSchemaProperty)
	var required []string
	queryMapping := make(map[string]string)
	bodyMapping := make(map[string]string)

	processParam := func(param *v3high.Parameter) {
		name := param.Name
		if name == "" {
			return
		}
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

	for _, param := range op.Parameters {
		processParam(param)
	}
	for _, param := range pathItemParams {
		processParam(param)
	}

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
			if s == nil {
				continue
			}

			scanSchemas := []*highbase.Schema{s}
			for _, alt := range s.OneOf {
				if altSchema := alt.Schema(); altSchema != nil && altSchema.Properties != nil {
					scanSchemas = append(scanSchemas, altSchema)
				}
			}
			for _, alt := range s.AnyOf {
				if altSchema := alt.Schema(); altSchema != nil && altSchema.Properties != nil {
					scanSchemas = append(scanSchemas, altSchema)
				}
			}

			for _, scan := range scanSchemas {
				if scan.Properties != nil {
					for prop := scan.Properties.First(); prop != nil; prop = prop.Next() {
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
				}
				for _, r := range scan.Required {
					if _, exists := properties[r]; exists {
						required = append(required, r)
					}
				}
			}
			break
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

		if len(s.Type) > 0 && s.Type[0] == "array" {
			return ""
		}

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
