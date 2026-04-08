// Package registry provides an embedded provider/model catalog sourced from
// models.dev. The JSON is embedded at build time via go:embed and parsed once
// at init, giving O(1) provider lookups and zero network latency.
package registry

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed models.json
var modelsJSON []byte

// Provider represents an LLM provider.
type Provider struct {
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	API    string           `json:"api,omitempty"`
	Doc    string           `json:"doc,omitempty"`
	Models map[string]Model `json:"models"`
}

// Model represents an LLM model.
type Model struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	Family           string      `json:"family,omitempty"`
	Reasoning        bool        `json:"reasoning,omitempty"`
	ToolCall         bool        `json:"tool_call,omitempty"`
	StructuredOutput bool        `json:"structured_output,omitempty"`
	OpenWeights      bool        `json:"open_weights,omitempty"`
	Knowledge        string      `json:"knowledge,omitempty"`
	ReleaseDate      string      `json:"release_date,omitempty"`
	Modalities       *Modalities `json:"modalities,omitempty"`
	Cost             *Cost       `json:"cost,omitempty"`
	Limit            *Limit      `json:"limit,omitempty"`
	Status           string      `json:"status,omitempty"`
}

// Modalities describes input/output modalities.
type Modalities struct {
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
}

// Cost holds per-million-token pricing.
type Cost struct {
	Input  float64 `json:"input,omitempty"`
	Output float64 `json:"output,omitempty"`
}

// Limit holds token limits.
type Limit struct {
	Context int64 `json:"context,omitempty"`
	Output  int64 `json:"output,omitempty"`
}

// Registry holds all providers and models, indexed for fast lookup.
type Registry struct {
	providers []Provider
	byID      map[string]*Provider
}

var (
	globalRegistry *Registry
	initOnce       sync.Once
)

func Global() *Registry {
	initOnce.Do(func() {
		globalRegistry = mustParse(modelsJSON)
	})
	return globalRegistry
}

func mustParse(data []byte) *Registry {
	var providers []Provider
	if err := json.Unmarshal(data, &providers); err != nil {
		panic("registry: failed to parse embedded models.json: " + err.Error())
	}
	return buildIndex(providers)
}

func buildIndex(providers []Provider) *Registry {
	r := &Registry{
		providers: providers,
		byID:      make(map[string]*Provider, len(providers)),
	}

	for i := range providers {
		p := &providers[i]
		r.byID[p.ID] = p
	}

	return r
}

func (r *Registry) GetProvider(id string) (*Provider, bool) {
	p, ok := r.byID[id]
	return p, ok
}

func (r *Registry) AllProviders() []Provider {
	return r.providers
}

func (r *Registry) ProviderCount() int {
	return len(r.providers)
}

func (r *Registry) ModelCount() int {
	n := 0
	for _, p := range r.providers {
		n += len(p.Models)
	}
	return n
}

// BestModelForForge selects the best model from a provider for forge system
// agents (or any task requiring a capable, cost-efficient model with tool
// calling). Reusable utility.
//
// Strategy:
//  1. Filter to models with tool_call=true, release_date >= 2025-11, not deprecated, has cost data.
//  2. If open weight models exist among candidates, pick the most recently released one.
//  3. Otherwise, score closed models by cost efficiency, context, and reasoning capability.
//  4. Fallback chain: relax date → relax to any tool_call model → first model.
func (r *Registry) BestModelForForge(providerID string) (string, bool) {
	provider, ok := r.byID[providerID]
	if !ok || len(provider.Models) == 0 {
		return "", false
	}

	// Try with strict date cutoff, then relax.
	for _, dateCutoff := range []string{"2025-11", "2025-06", ""} {
		if id, found := r.bestModelFiltered(provider, dateCutoff, true); found {
			return id, true
		}
	}

	// Last resort: any tool_call model regardless of date or cost.
	for _, dateCutoff := range []string{"2025-11", "2025-06", ""} {
		if id, found := r.bestModelFiltered(provider, dateCutoff, false); found {
			return id, true
		}
	}

	// Absolute last resort: first model in the provider.
	for id := range provider.Models {
		return id, true
	}
	return "", false
}

func (r *Registry) bestModelFiltered(provider *Provider, dateCutoff string, requireCost bool) (string, bool) {
	type candidate struct {
		id          string
		model       Model
		openWeights bool
		releaseDate string
	}

	var openCandidates, closedCandidates []candidate

	for id, model := range provider.Models {
		if !model.ToolCall {
			continue
		}
		if model.Status == "deprecated" {
			continue
		}
		if dateCutoff != "" && model.ReleaseDate < dateCutoff {
			continue
		}
		if requireCost && model.Cost == nil {
			continue
		}

		entry := candidate{id: id, model: model, openWeights: model.OpenWeights, releaseDate: model.ReleaseDate}
		if model.OpenWeights {
			openCandidates = append(openCandidates, entry)
		} else {
			closedCandidates = append(closedCandidates, entry)
		}
	}

	// Prefer open weight models: pick the most recently released.
	if len(openCandidates) > 0 {
		best := openCandidates[0]
		for _, c := range openCandidates[1:] {
			if c.releaseDate > best.releaseDate {
				best = c
			} else if c.releaseDate == best.releaseDate && c.id < best.id {
				best = c // deterministic tiebreak
			}
		}
		return best.id, true
	}

	// Closed models: score by cost efficiency, context, and reasoning.
	if len(closedCandidates) > 0 {
		type scored struct {
			id      string
			fitness float64
			date    string
		}
		var results []scored
		for _, c := range closedCandidates {
			fitness := closedModelFitness(c.model)
			results = append(results, scored{id: c.id, fitness: fitness, date: c.releaseDate})
		}
		best := results[0]
		for _, s := range results[1:] {
			if s.fitness > best.fitness {
				best = s
			} else if s.fitness == best.fitness && s.date > best.date {
				best = s
			} else if s.fitness == best.fitness && s.date == best.date && s.id < best.id {
				best = s
			}
		}
		return best.id, true
	}

	return "", false
}

// closedModelFitness scores a closed-weight model for forge suitability.
// Weights: 50% cost efficiency, 25% context window, 10% reasoning, 15% unused.
func closedModelFitness(model Model) float64 {
	// Context: linear up to 200k, capped at 1.0.
	var ctxScore float64
	if model.Limit != nil && model.Limit.Context > 0 {
		ctxScore = float64(model.Limit.Context) / 200000.0
		if ctxScore > 1.0 {
			ctxScore = 1.0
		}
	}

	// Cost: bell curve centered at $4/M output, half-width $20.
	var costScore float64
	if model.Cost != nil {
		dist := model.Cost.Output - 4.0
		if dist < 0 {
			dist = -dist
		}
		costScore = 1.0 - dist/20.0
		if costScore < 0 {
			costScore = 0
		}
	}

	// Reasoning bonus.
	var reasonScore float64
	if model.Reasoning {
		reasonScore = 1.0
	}

	return 0.25*ctxScore + 0.50*costScore + 0.10*reasonScore
}

