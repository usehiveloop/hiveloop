package handler

import (
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
)

type schemaPath struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type actionSchemaPaths struct {
	ResponseSchema string       `json:"response_schema"`
	Paths          []schemaPath `json:"paths"`
}

type schemaPathsResponse struct {
	Refs    map[string]string            `json:"refs"`
	Actions map[string]actionSchemaPaths `json:"actions"`
}

// GetSchemaPaths handles GET /v1/catalog/integrations/{id}/schema-paths.
// @Summary Get schema paths for an integration
// @Description Returns flattened schema property paths (up to 3 levels) for trigger refs and read action responses. Used for template autocomplete.
// @Tags integrations
// @Produce json
// @Param id path string true "Provider ID"
// @Success 200 {object} schemaPathsResponse
// @Failure 404 {object} errorResponse
// @Router /v1/catalog/integrations/{id}/schema-paths [get]
func (h *ActionsHandler) GetSchemaPaths(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	provider, ok := h.catalog.GetProvider(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "integration not found"})
		return
	}

	refsMap := make(map[string]string)
	if pt, ok := h.catalog.GetProviderTriggers(id); ok {
		for _, trigger := range pt.Triggers {
			for refName := range trigger.Refs {
				refsMap[refName] = "string"
			}
		}
	}
	if len(refsMap) == 0 {
		if pt, ok := h.catalog.GetProviderTriggersForVariant(id); ok {
			for _, trigger := range pt.Triggers {
				for refName := range trigger.Refs {
					refsMap[refName] = "string"
				}
			}
		}
	}

	actionPaths := make(map[string]actionSchemaPaths)
	schemas := provider.Schemas

	for actionKey, action := range provider.Actions {
		if action.Access != catalog.AccessRead || action.ResponseSchema == "" {
			continue
		}

		paths := flattenSchemaPaths(schemas, action.ResponseSchema, "", 3)
		actionPaths[actionKey] = actionSchemaPaths{
			ResponseSchema: action.ResponseSchema,
			Paths:          paths,
		}
	}

	writeJSON(w, http.StatusOK, schemaPathsResponse{
		Refs:    refsMap,
		Actions: actionPaths,
	})
}

func flattenSchemaPaths(schemas map[string]catalog.SchemaDefinition, schemaName, prefix string, maxDepth int) []schemaPath {
	if maxDepth <= 0 {
		return nil
	}

	schema, ok := schemas[schemaName]
	if !ok {
		return nil
	}

	if schema.Type == "array" {
		path := prefix
		if path == "" {
			path = schemaName
		}
		return []schemaPath{{Path: path, Type: "array"}}
	}

	var paths []schemaPath
	for propName, prop := range schema.Properties {
		fullPath := propName
		if prefix != "" {
			fullPath = prefix + "." + propName
		}

		paths = append(paths, schemaPath{Path: fullPath, Type: prop.Type})

		if prop.SchemaRef != "" && prop.Type == "object" {
			nested := flattenSchemaPaths(schemas, prop.SchemaRef, fullPath, maxDepth-1)
			paths = append(paths, nested...)
		}
	}

	sort.Slice(paths, func(i, j int) bool {
		return paths[i].Path < paths[j].Path
	})

	return paths
}
