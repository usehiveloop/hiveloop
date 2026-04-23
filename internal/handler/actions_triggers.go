package handler

import (
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/usehiveloop/hiveloop/internal/mcp/catalog"
)

type triggerSummary struct {
	Key           string            `json:"key"`
	DisplayName   string            `json:"display_name"`
	Description   string            `json:"description"`
	ResourceType  string            `json:"resource_type"`
	PayloadSchema string            `json:"payload_schema,omitempty"`
	Refs          map[string]string `json:"refs,omitempty"`
}

type triggersResponse struct {
	WebhookConfig *catalog.WebhookConfig `json:"webhook_config,omitempty"`
	Triggers      []triggerSummary       `json:"triggers"`
}

// ListActions handles GET /v1/catalog/integrations/{id}/actions.
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

// ListTriggers handles GET /v1/catalog/integrations/{id}/triggers.
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
