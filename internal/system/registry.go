package system

import (
	"fmt"
	"sort"
	"sync"
)

var (
	registryMu sync.RWMutex
	registry   = map[string]Task{}
)

// Register adds a task to the global registry. Called from each task file's
// init(). Panics if a task with the same name is already registered or if
// required fields are missing — both are programmer errors that should fail
// at startup, never at request time.
func Register(t Task) {
	if err := validateTask(t); err != nil {
		panic(fmt.Sprintf("system: invalid task %q: %v", t.Name, err))
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[t.Name]; exists {
		panic(fmt.Sprintf("system: duplicate task registration %q", t.Name))
	}
	registry[t.Name] = t
}

// Lookup returns the task registered under name, plus a presence bool.
func Lookup(name string) (Task, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	t, ok := registry[name]
	return t, ok
}

// All returns every registered task in name order. Intended for diagnostic
// listings (admin endpoints, logs); not on a hot path.
func All() []Task {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Task, 0, len(registry))
	for _, t := range registry {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ResetForTest clears the registry. Test-only — production code must not
// call this; the registry is build-time-stable.
func ResetForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = map[string]Task{}
}

func validateTask(t Task) error {
	if t.Name == "" {
		return fmt.Errorf("Name is required")
	}
	if t.Version == "" {
		return fmt.Errorf("Version is required (bump when prompt/schema/model changes)")
	}
	if t.ProviderGroup == "" {
		return fmt.Errorf("ProviderGroup is required")
	}
	if t.ModelTier == "" {
		return fmt.Errorf("ModelTier is required")
	}
	if t.ModelTier == ModelNamed && t.Model == "" {
		return fmt.Errorf("Model is required when ModelTier is ModelNamed")
	}
	if t.UserPromptTemplate == "" {
		return fmt.Errorf("UserPromptTemplate is required")
	}
	if t.MaxOutputTokens <= 0 {
		return fmt.Errorf("MaxOutputTokens must be > 0")
	}
	seen := map[string]struct{}{}
	for _, a := range t.Args {
		if a.Name == "" {
			return fmt.Errorf("ArgSpec.Name is required")
		}
		if _, dup := seen[a.Name]; dup {
			return fmt.Errorf("duplicate Args entry %q", a.Name)
		}
		seen[a.Name] = struct{}{}
		if a.Type == "" {
			return fmt.Errorf("ArgSpec %q: Type is required", a.Name)
		}
	}
	return nil
}
