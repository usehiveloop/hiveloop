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

