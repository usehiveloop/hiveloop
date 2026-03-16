package handler

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/llmvault/llmvault/internal/mcp/catalog"
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
	ID          string               `json:"id"`
	DisplayName string               `json:"display_name"`
	Resources   map[string]resource  `json:"resources"`
	Actions     []actionSummary      `json:"actions"`
}

type resource struct {
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	IDField     string `json:"id_field"`
	NameField   string `json:"name_field"`
	Icon        string `json:"icon,omitempty"`
}

type actionSummary struct {
	Key          string          `json:"key"`
	DisplayName  string          `json:"display_name"`
	Description  string          `json:"description"`
	Access       string          `json:"access"`
	ResourceType string          `json:"resource_type,omitempty"`
	Parameters   json.RawMessage `json:"parameters"`
}

// ListIntegrations handles GET /v1/integrations — returns all providers with action counts.
// @Summary List all integrations
// @Description Returns every integration provider in the catalog with action counts.
// @Tags integrations
// @Produce json
// @Success 200 {array} integrationSummary
// @Router /v1/integrations [get]
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

// GetIntegration handles GET /v1/integrations/{id} — returns provider detail with all actions.
// @Summary Get integration detail
// @Description Returns a single integration with its full action list.
// @Tags integrations
// @Produce json
// @Param id path string true "Provider ID (e.g. github-app, slack, jira)"
// @Success 200 {object} integrationDetail
// @Failure 404 {object} errorResponse
// @Router /v1/integrations/{id} [get]
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
		}
	}

	actions := actionsFromMap(p.Actions)

	writeJSON(w, http.StatusOK, integrationDetail{
		ID:          id,
		DisplayName: p.DisplayName,
		Resources:   resources,
		Actions:     actions,
	})
}

// ListActions handles GET /v1/integrations/{id}/actions — returns just the actions list.
// @Summary List actions for an integration
// @Description Returns all actions for a single integration, optionally filtered by access type.
// @Tags integrations
// @Produce json
// @Param id path string true "Provider ID"
// @Param access query string false "Filter by access type (read or write)"
// @Success 200 {array} actionSummary
// @Failure 404 {object} errorResponse
// @Router /v1/integrations/{id}/actions [get]
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

func actionsFromMap(m map[string]catalog.ActionDef) []actionSummary {
	actions := make([]actionSummary, 0, len(m))
	for key, a := range m {
		actions = append(actions, actionSummary{
			Key:          key,
			DisplayName:  a.DisplayName,
			Description:  a.Description,
			Access:       a.Access,
			ResourceType: a.ResourceType,
			Parameters:   a.Parameters,
		})
	}
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].Key < actions[j].Key
	})
	return actions
}
