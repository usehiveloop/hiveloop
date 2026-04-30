package handler

import (
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/model"
	"github.com/usehiveloop/hiveloop/internal/registry"
)

type ProviderHandler struct {
	reg *registry.Registry
	db  *gorm.DB
}

func NewProviderHandler(reg *registry.Registry, db *gorm.DB) *ProviderHandler {
	return &ProviderHandler{reg: reg, db: db}
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
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	ProviderID       string               `json:"provider_id,omitempty"`
	Family           string               `json:"family,omitempty"`
	Reasoning        bool                 `json:"reasoning,omitempty"`
	ToolCall         bool                 `json:"tool_call,omitempty"`
	StructuredOutput bool                 `json:"structured_output,omitempty"`
	OpenWeights      bool                 `json:"open_weights,omitempty"`
	Knowledge        string               `json:"knowledge,omitempty"`
	ReleaseDate      string               `json:"release_date,omitempty"`
	Modalities       *registry.Modalities `json:"modalities,omitempty"`
	Cost             *registry.Cost       `json:"cost,omitempty"`
	Limit            *registry.Limit      `json:"limit,omitempty"`
	Status           string               `json:"status,omitempty"`
	Speed            string               `json:"speed,omitempty"`
	Description      string               `json:"description,omitempty"`
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
			ModelCount: visibleModelCount(p.Models),
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func visibleModelCount(m map[string]registry.Model) int {
	n := 0
	for _, mdl := range m {
		if mdl.Hidden {
			continue
		}
		n++
	}
	return n
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

// @Summary List user-selectable models across providers
// @Description Returns models from providers that have at least one active system credential. Hidden routing-only models are excluded.
// @Tags providers
// @Produce json
// @Success 200 {array} modelSummary
// @Router /v1/models [get]
func (h *ProviderHandler) AllModels(w http.ResponseWriter, r *http.Request) {
	var providerIDs []string
	if err := h.db.WithContext(r.Context()).
		Model(&model.Credential{}).
		Where("is_system = ? AND revoked_at IS NULL", true).
		Distinct("provider_id").
		Pluck("provider_id", &providerIDs).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load system credentials"})
		return
	}

	out := make([]modelSummary, 0, 64)
	for _, providerID := range providerIDs {
		p, ok := h.reg.GetProvider(providerID)
		if !ok {
			continue
		}
		for _, mdl := range p.Models {
			if mdl.Hidden {
				continue
			}
			out = append(out, modelSummary{
				ID:               mdl.ID,
				Name:             mdl.Name,
				ProviderID:       p.ID,
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
				Speed:            mdl.Speed,
				Description:      mdl.Description,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	writeJSON(w, http.StatusOK, out)
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
		if mdl.Hidden {
			continue
		}
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
			Speed:            mdl.Speed,
			Description:      mdl.Description,
		})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models
}
