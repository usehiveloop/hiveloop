package handler

import (
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/ziraloop/ziraloop/internal/registry"
)

// ProviderHandler serves the embedded provider/model catalog.
type ProviderHandler struct {
	reg *registry.Registry
}

// NewProviderHandler creates a new provider handler.
func NewProviderHandler(reg *registry.Registry) *ProviderHandler {
	return &ProviderHandler{reg: reg}
}

// errorResponse is the standard error envelope.
type errorResponse struct {
	Error string `json:"error"`
}

type providerSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	API        string `json:"api,omitempty"`
	Doc        string `json:"doc,omitempty"`
	ModelCount int    `json:"model_count"`
}

type providerDetail struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	API    string         `json:"api,omitempty"`
	Doc    string         `json:"doc,omitempty"`
	Models []modelSummary `json:"models"`
}

type modelSummary struct {
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	Family           string              `json:"family,omitempty"`
	Reasoning        bool                `json:"reasoning,omitempty"`
	ToolCall         bool                `json:"tool_call,omitempty"`
	StructuredOutput bool                `json:"structured_output,omitempty"`
	OpenWeights      bool                `json:"open_weights,omitempty"`
	Knowledge        string              `json:"knowledge,omitempty"`
	ReleaseDate      string              `json:"release_date,omitempty"`
	Modalities       *registry.Modalities `json:"modalities,omitempty"`
	Cost             *registry.Cost       `json:"cost,omitempty"`
	Limit            *registry.Limit      `json:"limit,omitempty"`
	Status           string              `json:"status,omitempty"`
}

// List handles GET /v1/providers — returns all providers with model counts.
// @Summary List all providers
// @Description Returns every provider in the catalog with a model count.
// @Tags providers
// @Produce json
// @Success 200 {array} providerSummary
// @Router /v1/providers [get]
func (h *ProviderHandler) List(w http.ResponseWriter, r *http.Request) {
	all := h.reg.AllProviders()
	resp := make([]providerSummary, len(all))
	for i, p := range all {
		resp[i] = providerSummary{
			ID:         p.ID,
			Name:       p.Name,
			API:        p.API,
			Doc:        p.Doc,
			ModelCount: len(p.Models),
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// Get handles GET /v1/providers/{id} — returns provider detail with all models.
// @Summary Get provider detail
// @Description Returns a single provider with its full model list.
// @Tags providers
// @Produce json
// @Param id path string true "Provider ID"
// @Success 200 {object} providerDetail
// @Failure 404 {object} errorResponse
// @Router /v1/providers/{id} [get]
func (h *ProviderHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := h.reg.GetProvider(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
		return
	}

	models := modelsFromMap(p.Models)
	writeJSON(w, http.StatusOK, providerDetail{
		ID:     p.ID,
		Name:   p.Name,
		API:    p.API,
		Doc:    p.Doc,
		Models: models,
	})
}

// Models handles GET /v1/providers/{id}/models — returns just the models list.
// @Summary List models for a provider
// @Description Returns the model catalog for a single provider.
// @Tags providers
// @Produce json
// @Param id path string true "Provider ID"
// @Success 200 {array} modelSummary
// @Failure 404 {object} errorResponse
// @Router /v1/providers/{id}/models [get]
func (h *ProviderHandler) Models(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := h.reg.GetProvider(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
		return
	}

	writeJSON(w, http.StatusOK, modelsFromMap(p.Models))
}

func modelsFromMap(m map[string]registry.Model) []modelSummary {
	models := make([]modelSummary, 0, len(m))
	for _, mdl := range m {
		models = append(models, modelSummary{
			ID:               mdl.ID,
			Name:             mdl.Name,
			Family:           mdl.Family,
			Reasoning:        mdl.Reasoning,
			ToolCall:         mdl.ToolCall,
			StructuredOutput: mdl.StructuredOutput,
			OpenWeights:      mdl.OpenWeights,
			Knowledge:        mdl.Knowledge,
			ReleaseDate:      mdl.ReleaseDate,
			Modalities:       mdl.Modalities,
			Cost:             mdl.Cost,
			Limit:            mdl.Limit,
			Status:           mdl.Status,
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models
}
