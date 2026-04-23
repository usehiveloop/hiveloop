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

			if !useResources && !matchesTags(op, cfg.TagFilters) {
				continue
			}

			if isFileUpload(op) {
				continue
			}

			actionKey, displayName := deriveActionKey(op, method, path)
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
