// Package registry provides a hand-curated provider/model catalog. Each
// provider and model in the registry has been manually verified to work
// for autonomous agentic workflows (tool calling, structured output where
// applicable, recent releases, and tested cost/context characteristics).
//
// The catalog is defined as a Go literal in models.go rather than embedded
// JSON, so additions go through code review and the type checker enforces
// the schema. To add a model: edit models.go, run `go test ./internal/registry/...`
// to verify the registry still loads, then commit.
//
// This intentionally narrow allow-list replaces the previous approach of
// embedding the full models.dev catalog (1000+ models from 110+ providers),
// most of which we never tested.
package registry

import (
	"sort"
	"sync"
)

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
		globalRegistry = buildIndex(curatedProviders)
	})
	return globalRegistry
}

func buildIndex(providers []Provider) *Registry {
	// Defensive copy + alphabetical sort. The curated list in models.go
	// can be in any order; the public AllProviders() contract is sorted by
	// ID so the API responses and tests are stable.
	sorted := make([]Provider, len(providers))
	copy(sorted, providers)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	r := &Registry{
		providers: sorted,
		byID:      make(map[string]*Provider, len(sorted)),
	}

	for i := range sorted {
		p := &sorted[i]
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


