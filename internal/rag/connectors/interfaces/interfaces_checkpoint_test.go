package interfaces

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/usehiveloop/hiveloop/internal/nango"
)

// ---------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------

// stubSource is a test-only Source implementation.
type stubSource struct {
	id, orgID, kind string
	cfg             json.RawMessage
}

func (s *stubSource) SourceID() string        { return s.id }
func (s *stubSource) OrgID() string           { return s.orgID }
func (s *stubSource) SourceKind() string      { return s.kind }
func (s *stubSource) Config() json.RawMessage { return s.cfg }

// stubConnector implements Connector for registry tests.
type stubConnector struct{ kind string }

func (c *stubConnector) Kind() string                                     { return c.kind }
func (c *stubConnector) ValidateConfig(_ context.Context, _ Source) error { return nil }

// testCheckpoint is a Checkpoint-satisfying type used by
// TestCheckpoint_MarkerInterfaceCompiles.
type testCheckpoint struct {
	Cursor string `json:"cursor"`
	Page   int    `json:"page"`
}

func (testCheckpoint) isCheckpoint() {}

// testCheckpointedConnector implements CheckpointedConnector[testCheckpoint].
// Used only to prove at compile time that the generic constraint works.
// All methods use pointer receivers to match stubConnector's receiver set.
type testCheckpointedConnector struct{ stubConnector }

func (*testCheckpointedConnector) LoadFromCheckpoint(
	_ context.Context, _ Source, _ testCheckpoint, _, _ time.Time,
) (<-chan DocumentOrFailure, error) {
	ch := make(chan DocumentOrFailure)
	close(ch)
	return ch, nil
}
func (*testCheckpointedConnector) DummyCheckpoint() testCheckpoint { return testCheckpoint{} }
func (*testCheckpointedConnector) UnmarshalCheckpoint(raw json.RawMessage) (testCheckpoint, error) {
	var cp testCheckpoint
	if err := json.Unmarshal(raw, &cp); err != nil {
		return testCheckpoint{}, err
	}
	return cp, nil
}

// takesCheckpointedConnector is a package-local generic function that
// accepts any CheckpointedConnector[T]. Its sole purpose is to prove
// via compile-time that the Checkpoint marker-interface constraint
// composes correctly with real generic consumers (tranche 3C's
// scheduler will look like this).
func takesCheckpointedConnector[T Checkpoint](_ CheckpointedConnector[T]) {}

// ---------------------------------------------------------------------
// 1. DocumentOrFailure constructor invariants
// ---------------------------------------------------------------------

func TestCheckpoint_MarkerInterfaceCompiles(t *testing.T) {
	// If this test body compiles, the constraint is satisfied — the
	// body then verifies runtime behavior on top of that.
	var conn CheckpointedConnector[testCheckpoint] = &testCheckpointedConnector{
		stubConnector: stubConnector{kind: "test-cp"},
	}
	takesCheckpointedConnector[testCheckpoint](conn)

	// Unmarshal round-trip via the connector's method.
	raw := json.RawMessage(`{"cursor":"abc","page":7}`)
	cp, err := conn.UnmarshalCheckpoint(raw)
	if err != nil {
		t.Fatalf("UnmarshalCheckpoint: %v", err)
	}
	if cp.Cursor != "abc" || cp.Page != 7 {
		t.Fatalf("unmarshal mismatch: %+v", cp)
	}

	// AnyCheckpoint also satisfies the constraint — sanity-check with
	// another concrete type so we don't accidentally depend on
	// testCheckpoint specifics.
	takesAnyCheckpoint(AnyCheckpoint{HasMore: true})
}

func takesAnyCheckpoint[T Checkpoint](_ T) {}

// ---------------------------------------------------------------------
// 12. Factory signature composes with a zero-value Source
// ---------------------------------------------------------------------

func TestFactorySignature_AcceptsRealRAGSource(t *testing.T) {
	// DEVIATION: the brief says "construct a zero-value RAGSource".
	// Per connector.go's Source interface comment, we use the local
	// Source interface rather than ragmodel.RAGSource (Tranche 3A).
	// Tranche 3A's concrete struct will satisfy Source; until then,
	// we exercise the signature with a zero-value stubSource — this
	// proves the Factory type composes correctly with something that
	// has the Source shape, which is what the test is really asserting.
	var src Source = &stubSource{}

	factory := func(s Source, n *nango.Client) (Connector, error) {
		// Prove the factory can read from the zero-value Source
		// without panicking (nil Config, empty kind).
		_ = s.Config()
		_ = s.SourceKind()
		_ = n
		return &stubConnector{kind: "ok"}, nil
	}

	c, err := factory(src, nil)
	if err != nil {
		t.Fatalf("factory(zero Source) failed: %v", err)
	}
	if c == nil {
		t.Fatalf("factory returned nil Connector without error")
	}
}

// ---------------------------------------------------------------------
// Union-type constructor coverage
//
// The Slim / Access / Group Or-Failure unions share the same discipline
// as DocumentOrFailure: exactly one field non-nil, constructor panics
// on nil. We test them together here because the per-union behavior is
// identical modulo type.
// ---------------------------------------------------------------------

func TestSlimOrFailure_ConstructorsEnforceInvariant(t *testing.T) {
	slim := &SlimDocument{DocID: "x"}
	r := NewSlimResult(slim)
	if r.Slim != slim || r.Failure != nil {
		t.Fatalf("NewSlimResult: %+v", r)
	}

	f := &ConnectorFailure{FailureMessage: "boom"}
	ff := NewSlimFailure(f)
	if ff.Failure != f || ff.Slim != nil {
		t.Fatalf("NewSlimFailure: %+v", ff)
	}

	assertPanic(t, "NewSlimResult(nil)", func() { NewSlimResult(nil) })
	assertPanic(t, "NewSlimFailure(nil)", func() { NewSlimFailure(nil) })
}

func TestDocExternalAccessOrFailure_ConstructorsEnforceInvariant(t *testing.T) {
	a := &DocExternalAccess{DocID: "x", ExternalAccess: &ExternalAccess{IsPublic: true}}
	r := NewAccessResult(a)
	if r.Access != a || r.Failure != nil {
		t.Fatalf("NewAccessResult: %+v", r)
	}

	f := &ConnectorFailure{FailureMessage: "boom"}
	ff := NewAccessFailure(f)
	if ff.Failure != f || ff.Access != nil {
		t.Fatalf("NewAccessFailure: %+v", ff)
	}

	assertPanic(t, "NewAccessResult(nil)", func() { NewAccessResult(nil) })
	assertPanic(t, "NewAccessFailure(nil)", func() { NewAccessFailure(nil) })
}

func TestExternalGroupOrFailure_ConstructorsEnforceInvariant(t *testing.T) {
	g := &ExternalGroup{GroupID: "external_group:github_acme_backend"}
	r := NewGroupResult(g)
	if r.Group != g || r.Failure != nil {
		t.Fatalf("NewGroupResult: %+v", r)
	}

	f := &ConnectorFailure{FailureMessage: "boom"}
	ff := NewGroupFailure(f)
	if ff.Failure != f || ff.Group != nil {
		t.Fatalf("NewGroupFailure: %+v", ff)
	}

	assertPanic(t, "NewGroupResult(nil)", func() { NewGroupResult(nil) })
	assertPanic(t, "NewGroupFailure(nil)", func() { NewGroupFailure(nil) })
}

// TestRegistry_PanicsOnEmptyKindOrNilFactory pins the two other Register
// failure modes documented in the godoc. Without these, Register's
// branchful validation would be uncovered.
func TestRegistry_PanicsOnEmptyKindOrNilFactory(t *testing.T) {
	t.Cleanup(resetRegistryForTest)
	resetRegistryForTest()

	assertPanic(t, "Register empty kind", func() {
		Register("", func(_ Source, _ *nango.Client) (Connector, error) { return nil, nil })
	})
	assertPanic(t, "Register nil factory", func() {
		Register("some-kind", nil)
	})
}

// ---------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------

func assertPanic(t *testing.T, label string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("%s: expected panic, got none", label)
		}
	}()
	fn()
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
