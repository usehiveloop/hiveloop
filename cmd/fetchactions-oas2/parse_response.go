package main

import (
	"strings"

	highbase "github.com/pb33f/libopenapi/datamodel/high/base"
	v2high "github.com/pb33f/libopenapi/datamodel/high/v2"
)

// extractV2ResponseSchema looks at the 200 response of a Swagger 2.0 operation,
// resolves the schema ref name, flattens it into the schemas map, and returns
// the ref name. Returns "" if no usable response schema is found.
func extractV2ResponseSchema(op *v2high.Operation, definitions map[string]*highbase.SchemaProxy, schemas map[string]FlatSchema) string {
	if op.Responses == nil || op.Responses.Codes == nil {
		return ""
	}

	for _, code := range []string{"200", "201"} {
		resp := op.Responses.Codes.GetOrZero(code)
		if resp == nil || resp.Schema == nil {
			continue
		}

		schema := resp.Schema.Schema()
		if schema == nil {
			continue
		}

		refName := extractRefName(resp.Schema)
		if refName != "" {
			flattenV2Definition(refName, definitions, schemas)
			return refName
		}

		if len(schema.Type) > 0 && schema.Type[0] == "array" && schema.Items != nil && schema.Items.A != nil {
			itemRef := extractRefName(schema.Items.A)
			if itemRef != "" {
				arraySchemaName := itemRef + "_list"
				if _, exists := schemas[arraySchemaName]; !exists {
					flattenV2Definition(itemRef, definitions, schemas)
					schemas[arraySchemaName] = FlatSchema{
						Type:  "array",
						Items: &FlatSchemaRef{Ref: itemRef},
					}
				}
				return arraySchemaName
			}
		}

		if len(schema.Type) > 0 && schema.Type[0] == "object" && schema.Properties != nil {
			inlineName := toSnakeCase(op.OperationId) + "_response"
			if _, exists := schemas[inlineName]; !exists {
				flat := FlatSchema{
					Type:       "object",
					Properties: make(map[string]SchemaProperty),
				}
				for prop := schema.Properties.First(); prop != nil; prop = prop.Next() {
					propName := prop.Key()
					propProxy := prop.Value()
					flat.Properties[propName] = flattenV2Property(propProxy)
				}
				schemas[inlineName] = flat
			}
			return inlineName
		}
	}

	return ""
}

// extractRefName pulls the definition name from a schema proxy's $ref if present.
func extractRefName(proxy *highbase.SchemaProxy) string {
	if proxy == nil {
		return ""
	}
	ref := proxy.GetReference()
	if ref == "" {
		return ""
	}
	const prefix = "#/definitions/"
	if strings.HasPrefix(ref, prefix) {
		return strings.TrimPrefix(ref, prefix)
	}
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

// flattenV2Definition resolves a Swagger 2.0 definition by name and stores its
// flattened top-level properties in the schemas map. Nested $refs are collapsed
// to their type only (no recursion).
func flattenV2Definition(name string, definitions map[string]*highbase.SchemaProxy, schemas map[string]FlatSchema) {
	if _, exists := schemas[name]; exists {
		return
	}

	proxy, ok := definitions[name]
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

	if flat.Type == "array" && schema.Items != nil && schema.Items.A != nil {
		itemRef := extractRefName(schema.Items.A)
		if itemRef != "" {
			flattenV2Definition(itemRef, definitions, schemas)
			flat.Items = &FlatSchemaRef{Ref: itemRef}
		}
		schemas[name] = flat
		return
	}

	if schema.Properties != nil {
		for prop := schema.Properties.First(); prop != nil; prop = prop.Next() {
			propName := prop.Key()
			propProxy := prop.Value()
			flat.Properties[propName] = flattenV2Property(propProxy)
		}
	}

	schemas[name] = flat

	for _, propDef := range flat.Properties {
		if propDef.SchemaRef != "" {
			flattenV2Definition(propDef.SchemaRef, definitions, schemas)
		}
	}
}

// flattenV2Property extracts type + description from a schema proxy without recursing.
// When the property is a $ref to a named definition, the ref name is preserved
// as SchemaRef so the frontend can resolve nested object types.
func flattenV2Property(proxy *highbase.SchemaProxy) SchemaProperty {
	prop := SchemaProperty{Type: "string"}
	if proxy == nil {
		return prop
	}

	refName := extractRefName(proxy)

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
