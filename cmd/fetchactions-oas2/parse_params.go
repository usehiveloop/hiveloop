package main

import (
	"encoding/json"

	v2high "github.com/pb33f/libopenapi/datamodel/high/v2"
)

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
		if param.In == "formData" {
			return
		}
		if _, exists := properties[name]; exists {
			return
		}

		switch param.In {
		case "body":
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
