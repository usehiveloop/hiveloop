package handler

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/ziraloop/ziraloop/internal/mcp/catalog"
)

// ActionsHandler serves the embedded actions catalog.
type ActionsHandler struct {
	catalog *catalog.Catalog
}

// NewActionsHandler creates a new actions handler.
func NewActionsHandler(c *catalog.Catalog) *ActionsHandler {
	return &ActionsHandler{catalog: c}
}

type integrationSummary struct {
	ID           string `json:"id"`
	DisplayName  string `json:"display_name"`
	ActionCount  int    `json:"action_count"`
	ReadCount    int    `json:"read_count"`
	WriteCount   int    `json:"write_count"`
	HasResources bool   `json:"has_resources"`
}

type integrationDetail struct {
	ID          string                          `json:"id"`
	DisplayName string                          `json:"display_name"`
	Resources   map[string]resource             `json:"resources"`
	Actions     []actionSummary                 `json:"actions"`
	Schemas     map[string]catalog.SchemaDefinition `json:"schemas,omitempty"`
}

type resource struct {
	DisplayName string            `json:"display_name"`
	Description string            `json:"description"`
	IDField     string            `json:"id_field"`
	NameField   string            `json:"name_field"`
	Icon        string            `json:"icon,omitempty"`
	RefBindings map[string]string `json:"ref_bindings,omitempty"`
}

type actionSummary struct {
	Key            string          `json:"key"`
	DisplayName    string          `json:"display_name"`
	Description    string          `json:"description"`
	Access         string          `json:"access"`
	ResourceType   string          `json:"resource_type,omitempty"`
	Parameters     json.RawMessage `json:"parameters"`
	ResponseSchema string          `json:"response_schema,omitempty"`
}

// ListIntegrations handles GET /v1/catalog/integrations — returns all providers with action counts.
// @Summary List all integrations
// @Description Returns every integration provider in the catalog with action counts.
// @Tags integrations
// @Produce json
// @Success 200 {array} integrationSummary
// @Router /v1/catalog/integrations [get]
func (h *ActionsHandler) ListIntegrations(w http.ResponseWriter, r *http.Request) {
	names := h.catalog.ListProviders()
	resp := make([]integrationSummary, 0, len(names))

	for _, name := range names {
		p, ok := h.catalog.GetProvider(name)
		if !ok {
			continue
		}

		reads := 0
		writes := 0
		for _, a := range p.Actions {
			switch a.Access {
			case catalog.AccessRead:
				reads++
			case catalog.AccessWrite:
				writes++
			}
		}

		resp = append(resp, integrationSummary{
			ID:           name,
			DisplayName:  p.DisplayName,
			ActionCount:  len(p.Actions),
			ReadCount:    reads,
			WriteCount:   writes,
			HasResources: len(p.Resources) > 0,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetIntegration handles GET /v1/catalog/integrations/{id} — returns provider detail with all actions.
// @Summary Get integration detail
// @Description Returns a single integration with its full action list.
// @Tags integrations
// @Produce json
// @Param id path string true "Provider ID (e.g. github-app, slack, jira)"
// @Success 200 {object} integrationDetail
// @Failure 404 {object} errorResponse
// @Router /v1/catalog/integrations/{id} [get]
func (h *ActionsHandler) GetIntegration(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := h.catalog.GetProvider(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "integration not found"})
		return
	}

	resources := make(map[string]resource, len(p.Resources))
	for k, r := range p.Resources {
		resources[k] = resource{
			DisplayName: r.DisplayName,
			Description: r.Description,
			IDField:     r.IDField,
			NameField:   r.NameField,
			Icon:        r.Icon,
			RefBindings: r.RefBindings,
		}
	}

	actions := actionsFromMap(p.Actions)

	writeJSON(w, http.StatusOK, integrationDetail{
		ID:          id,
		DisplayName: p.DisplayName,
		Resources:   resources,
		Actions:     actions,
		Schemas:     p.Schemas,
	})
}

// ListActions handles GET /v1/catalog/integrations/{id}/actions — returns just the actions list.
// @Summary List actions for an integration
// @Description Returns all actions for a single integration, optionally filtered by access type.
// @Tags integrations
// @Produce json
// @Param id path string true "Provider ID"
// @Param access query string false "Filter by access type (read or write)"
// @Success 200 {array} actionSummary
// @Failure 404 {object} errorResponse
// @Router /v1/catalog/integrations/{id}/actions [get]
func (h *ActionsHandler) ListActions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := h.catalog.GetProvider(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "integration not found"})
		return
	}

	accessFilter := r.URL.Query().Get("access")
	actions := actionsFromMap(p.Actions)

	if accessFilter != "" {
		filtered := make([]actionSummary, 0, len(actions))
		for _, a := range actions {
			if a.Access == accessFilter {
				filtered = append(filtered, a)
			}
		}
		actions = filtered
	}

	writeJSON(w, http.StatusOK, actions)
}

type triggerSummary struct {
	Key           string            `json:"key"`
	DisplayName   string            `json:"display_name"`
	Description   string            `json:"description"`
	ResourceType  string            `json:"resource_type"`
	PayloadSchema string            `json:"payload_schema,omitempty"`
	Refs          map[string]string `json:"refs,omitempty"` // ref_name → dot-path into payload
}

type triggersResponse struct {
	WebhookConfig *catalog.WebhookConfig `json:"webhook_config,omitempty"`
	Triggers      []triggerSummary       `json:"triggers"`
}

// ListTriggers handles GET /v1/catalog/integrations/{id}/triggers — returns webhook triggers for a provider.
// @Summary List triggers for an integration
// @Description Returns all webhook event triggers for a single integration, including manual webhook configuration requirements if applicable.
// @Tags integrations
// @Produce json
// @Param id path string true "Provider ID"
// @Success 200 {object} triggersResponse
// @Failure 404 {object} errorResponse
// @Router /v1/catalog/integrations/{id}/triggers [get]
func (h *ActionsHandler) ListTriggers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	pt, ok := h.catalog.GetProviderTriggers(id)
	if !ok {
		// Try base provider name (e.g., "github" for "github-app").
		pt, ok = h.catalog.GetProviderTriggersForVariant(id)
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "no triggers found for this integration"})
		return
	}

	triggers := make([]triggerSummary, 0, len(pt.Triggers))
	for key, trigger := range pt.Triggers {
		triggers = append(triggers, triggerSummary{
			Key:           key,
			DisplayName:   trigger.DisplayName,
			Description:   trigger.Description,
			ResourceType:  trigger.ResourceType,
			PayloadSchema: trigger.PayloadSchema,
			Refs:          trigger.Refs,
		})
	}
	sort.Slice(triggers, func(i, j int) bool {
		return triggers[i].Key < triggers[j].Key
	})

	writeJSON(w, http.StatusOK, triggersResponse{
		WebhookConfig: pt.WebhookConfig,
		Triggers:      triggers,
	})
}

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

// GetSchemaPaths handles GET /v1/catalog/integrations/{id}/schema-paths — returns flattened schema paths for autocomplete.
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

	// Build refs map (ref_name → type) by merging refs from all triggers.
	refsMap := make(map[string]string)
	if pt, ok := h.catalog.GetProviderTriggers(id); ok {
		for _, trigger := range pt.Triggers {
			for refName := range trigger.Refs {
				refsMap[refName] = "string" // refs are always scalar strings/numbers extracted from payload
			}
		}
	}
	if len(refsMap) == 0 {
		// Try variant.
		if pt, ok := h.catalog.GetProviderTriggersForVariant(id); ok {
			for _, trigger := range pt.Triggers {
				for refName := range trigger.Refs {
					refsMap[refName] = "string"
				}
			}
		}
	}

	// Build action paths for all read actions.
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

// flattenSchemaPaths walks a schema definition recursively up to maxDepth levels
// and returns all reachable property paths with their types.
func flattenSchemaPaths(schemas map[string]catalog.SchemaDefinition, schemaName, prefix string, maxDepth int) []schemaPath {
	if maxDepth <= 0 {
		return nil
	}

	schema, ok := schemas[schemaName]
	if !ok {
		return nil
	}

	// For array types, return a single entry for the array itself.
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

		// If the property references another schema, recurse.
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

func actionsFromMap(m map[string]catalog.ActionDef) []actionSummary {
	actions := make([]actionSummary, 0, len(m))
	for key, a := range m {
		actions = append(actions, actionSummary{
			Key:          key,
			DisplayName:  a.DisplayName,
			Description:  a.Description,
			Access:         a.Access,
			ResourceType:   a.ResourceType,
			Parameters:     a.Parameters,
			ResponseSchema: a.ResponseSchema,
		})
	}
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Key < actions[j].Key
	})
	return actions
}
