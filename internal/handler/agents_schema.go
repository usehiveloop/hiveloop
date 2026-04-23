package handler

import (
	"fmt"

	"github.com/usehiveloop/hiveloop/internal/model"
)

func validateJSONSchema(agentConfig model.JSON) string {
	if agentConfig == nil {
		return ""
	}

	raw, ok := agentConfig["json_schema"]
	if !ok || raw == nil {
		return ""
	}

	schema, ok := raw.(map[string]any)
	if !ok {
		return "json_schema must be an object"
	}

	name, _ := schema["name"].(string)
	if name == "" {
		return "json_schema.name is required and must be a non-empty string"
	}

	schemaDef, ok := schema["schema"].(map[string]any)
	if !ok {
		return "json_schema.schema is required and must be an object"
	}

	schemaType, _ := schemaDef["type"].(string)
	if schemaType != "object" {
		return "json_schema.schema.type must be \"object\""

	}

	if err := validateSchemaDepthAndProperties(schemaDef, 1, new(int)); err != "" {
		return err
	}

	if err := validateSchemaKeywords(schemaDef); err != "" {
		return err
	}

	return ""
}

func validateSchemaDepthAndProperties(schema map[string]any, depth int, propCount *int) string {
	if depth > 5 {
		return "json_schema.schema exceeds maximum nesting depth of 5"
	}

	props, _ := schema["properties"].(map[string]any)
	*propCount += len(props)
	if *propCount > 100 {
		return "json_schema.schema exceeds maximum of 100 total properties"
	}

	for _, v := range props {
		if obj, ok := v.(map[string]any); ok {
			propType, _ := obj["type"].(string)
			if propType == "object" {
				if err := validateSchemaDepthAndProperties(obj, depth+1, propCount); err != "" {
					return err
				}
			}
			if propType == "array" {
				if items, ok := obj["items"].(map[string]any); ok {
					itemType, _ := items["type"].(string)
					if itemType == "object" {
						if err := validateSchemaDepthAndProperties(items, depth+1, propCount); err != "" {
							return err
						}
					}
				}
			}
		}
	}
	return ""
}

func validateSchemaKeywords(schema map[string]any) string {
	rejected := []string{"$ref", "$defs", "oneOf", "allOf", "not", "if", "then", "else",
		"pattern", "format", "minLength", "maxLength", "minimum", "maximum",
		"minItems", "maxItems", "patternProperties"}

	return walkSchemaKeywords(schema, rejected)
}

func walkSchemaKeywords(obj map[string]any, rejected []string) string {
	for _, kw := range rejected {
		if _, exists := obj[kw]; exists {
			return fmt.Sprintf("json_schema.schema contains unsupported keyword %q (not portable across providers)", kw)
		}
	}
	if props, ok := obj["properties"].(map[string]any); ok {
		for _, v := range props {
			if sub, ok := v.(map[string]any); ok {
				if err := walkSchemaKeywords(sub, rejected); err != "" {
					return err
				}
			}
		}
	}
	if items, ok := obj["items"].(map[string]any); ok {
		if err := walkSchemaKeywords(items, rejected); err != "" {
			return err
		}
	}
	if anyOf, ok := obj["anyOf"].([]any); ok {
		for _, item := range anyOf {
			if sub, ok := item.(map[string]any); ok {
				if err := walkSchemaKeywords(sub, rejected); err != "" {
					return err
				}
			}
		}
	}
	return ""
}
