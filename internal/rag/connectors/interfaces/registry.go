package interfaces

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/usehiveloop/hiveloop/internal/nango"
)

// Factory constructs a Connector instance bound to a specific Source
// (a RAGSource row) and a shared Nango client. Each connector package
// (e.g. github in Tranche 3D) exports one Factory and registers it in
// the factory registry below via Register in its init() function.
//
// The returned Connector may additionally satisfy
// CheckpointedConnector[T], PermSyncConnector, and/or SlimConnector —
// callers type-assert for the capabilities they need.
//
// DEVIATION: the brief specifies `*ragmodel.RAGSource` for the src
// parameter. See connector.go's Source interface comment for the
// rationale — we use the local Source interface to keep this tranche
// independent of Tranche 3A's concrete struct.
type Factory func(src Source, nango *nango.Client) (Connector, error)

// ErrUnknownKind is returned from Lookup when no factory is registered
// for the requested kind. Callers can distinguish "no such connector"
// from other errors via errors.Is(err, ErrUnknownKind).
var ErrUnknownKind = errors.New("connector: unknown kind")

var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}
)

// Register associates a Factory with a connector kind. Called from
// each connector package's init() function.
//
// Panics on duplicate registration — a connector package being
// imported twice into the same binary under the same kind is a
// programming error that can't be recovered from at runtime (two
// factories for one kind creates non-determinism in Lookup).
// Matches the panic-on-duplicate pattern used elsewhere in the
// codebase (database/sql.Register, expvar.Publish, etc.).
//
// Kind must be non-empty. Empty kinds would collide with the "unset"
// default and break admin-UI enumeration.
func Register(kind string, f Factory) {
	if kind == "" {
		panic("connector: Register called with empty kind")
	}
	if f == nil {
		panic(fmt.Sprintf("connector: Register called with nil factory for kind %q", kind))
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := registry[kind]; exists {
		panic(fmt.Sprintf("connector: duplicate registration for kind %q", kind))
	}
	registry[kind] = f
}

// Lookup returns the Factory registered for the given kind, or
// ErrUnknownKind if none is registered. Safe for concurrent use.
func Lookup(kind string) (Factory, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	f, ok := registry[kind]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKind, kind)
	}
	return f, nil
}

// RegisteredKinds returns the alphabetically sorted list of registered
// connector kinds. Used by the admin UI's "Add RAG source" picker
// (Tranche 3E) and by tests to assert the expected set of connectors
// is linked into the binary.
//
// Sorted output is contractual — callers must not need to sort
// themselves, and deterministic output simplifies golden-file
// comparisons.
func RegisteredKinds() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	kinds := make([]string, 0, len(registry))
	for k := range registry {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

// resetRegistryForTest clears the registry. Only callable from tests
// within this package (lower-case exported name is a Go package-private
// escape hatch). Without this, tests that exercise Register would leak
// state across tests and across packages.
func resetRegistryForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = map[string]Factory{}
}
