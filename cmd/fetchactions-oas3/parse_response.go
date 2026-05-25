package main

import (
	"strings"

	highbase "github.com/pb33f/libopenapi/datamodel/high/base"
	v3high "github.com/pb33f/libopenapi/datamodel/high/v3"
)

// extractV3ResponseSchema looks at the 200/201 response of an OpenAPI 3.x operation,
// resolves the schema ref, flattens it into the schemas map, and returns the ref name.
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

		refName := extractV3RefName(proxy)
		if refName != "" {
			flattenV3Component(refName, components, schemas)
			return refName
		}

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
