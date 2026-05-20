package hivy

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
)

// ToolHandler processes a single tool call from the LLM. It returns a
// text result (shown to the LLM on the next turn) and a done flag.
// When done is true, the agent loop stops and collects results.
type ToolHandler func(ctx context.Context, callID string, args json.RawMessage) (result string, done bool, err error)

// PlannedEnrichment records one context-gathering step the LLM planned.
// The executor resolves template references and executes the actual fetch.
type PlannedEnrichment struct {
	ConnectionID uuid.UUID      `json:"connection_id"`
	As           string         `json:"as"`
	Action       string         `json:"action"`
	Params       map[string]any `json:"params"`
}

// PlannedStepRegistry tracks enrichment steps planned so far for validating
// {{step.field}} references and uniqueness of `as`.
type PlannedStepRegistry struct {
	mu    sync.Mutex
	steps map[string]string // as → action key (for existence check)
	order []string          // insertion order
}

// NewPlannedStepRegistry creates an empty registry.
func NewPlannedStepRegistry() *PlannedStepRegistry {
	return &PlannedStepRegistry{steps: make(map[string]string)}
}

// Has returns true if a step with the given name has been planned.
func (r *PlannedStepRegistry) Has(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.steps[name]
	return ok
}

// Add registers a step. Returns false if the name is already taken.
func (r *PlannedStepRegistry) Add(name, action string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.steps[name]; ok {
		return false
	}
	r.steps[name] = action
	r.order = append(r.order, name)
	return true
}

// Names returns the planned step names in order.
func (r *PlannedStepRegistry) Names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]string, len(r.order))
	copy(result, r.order)
	return result
}

// ConnectionWithActions pairs a connection with its provider's read actions.
type ConnectionWithActions struct {
	Connection  model.InConnection
	Provider    string
	ReadActions map[string]catalog.ActionDef // keyed by action key
}
