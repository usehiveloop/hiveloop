// Package registry provides an embedded provider/model catalog sourced from
// models.dev. The JSON is embedded at build time via go:embed and parsed once
// at init, giving O(1) provider lookups and zero network latency.
package registry

import (
	_ "embed"
	"encoding/json"
	"net/url"
	"strings"
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
	byHost    map[string]*Provider // hostname → provider (for URL matching)
}

// knownHosts maps API hostnames to provider IDs for providers that lack an
// `api` field in the models.dev data.
var knownHosts = map[string]string{
	"api.openai.com":                       "openai",
	"api.anthropic.com":                    "anthropic",
	"generativelanguage.googleapis.com":    "google",
	"api.groq.com":                         "groq",
	"api.mistral.ai":                       "mistral",
	"api.cohere.com":                       "cohere",
	"api.fireworks.ai":                     "fireworks-ai",
	"api.together.xyz":                     "togetherai",
	"api.perplexity.ai":                    "perplexity",
	"inference.cerebras.ai":                "cerebras",
	"api.x.ai":                             "xai",
	"api.novita.ai":                        "novita-ai",
	"api-inference.huggingface.co":         "huggingface",
	"api.deepinfra.com":                    "deepinfra",
	"api.upstage.ai":                       "upstage",
	"api.friendli.ai":                      "friendli",
	"api.baseten.co":                       "baseten",
	"integrate.api.nvidia.com":             "nvidia",
	"models.inference.ai.azure.com":        "azure",
}

var (
	globalRegistry *Registry
	initOnce       sync.Once
)

// Global returns the singleton registry, parsing the embedded JSON on first call.
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
		byHost:    make(map[string]*Provider, len(providers)*2),
	}

	for i := range providers {
		p := &providers[i]
		r.byID[p.ID] = p

		// Index by API hostname for URL-based matching.
		if p.API != "" {
			if u, err := url.Parse(p.API); err == nil && u.Host != "" {
				host := strings.TrimPrefix(u.Host, "www.")
				r.byHost[host] = p
			}
		}
	}

	// Add well-known hostnames for providers that lack an `api` field.
	for host, providerID := range knownHosts {
		if p, ok := r.byID[providerID]; ok {
			if _, exists := r.byHost[host]; !exists {
				r.byHost[host] = p
			}
		}
	}

	return r
}

// GetProvider returns a provider by its ID (e.g. "openai", "anthropic").
func (r *Registry) GetProvider(id string) (*Provider, bool) {
	p, ok := r.byID[id]
	return p, ok
}

// MatchByBaseURL attempts to identify a provider from a credential's base_url.
// It matches the hostname against known provider API endpoints.
func (r *Registry) MatchByBaseURL(baseURL string) (*Provider, bool) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return nil, false
	}

	host := strings.TrimPrefix(u.Host, "www.")

	// Direct hostname match.
	if p, ok := r.byHost[host]; ok {
		return p, true
	}

	return nil, false
}

// AllProviders returns all providers (sorted by ID from build time).
func (r *Registry) AllProviders() []Provider {
	return r.providers
}

// ProviderCount returns the total number of providers.
func (r *Registry) ProviderCount() int {
	return len(r.providers)
}

// ModelCount returns the total number of models across all providers.
func (r *Registry) ModelCount() int {
	n := 0
	for _, p := range r.providers {
		n += len(p.Models)
	}
	return n
}

