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

func TestDocumentOrFailure_ConstructorsAreMutuallyExclusive(t *testing.T) {
	doc := &Document{DocID: "x"}
	res := NewDocResult(doc)
	if res.Doc != doc {
		t.Fatalf("NewDocResult: Doc = %v, want %v", res.Doc, doc)
	}
	if res.Failure != nil {
		t.Fatalf("NewDocResult: Failure should be nil, got %v", res.Failure)
	}

	f := &ConnectorFailure{FailureMessage: "boom"}
	fail := NewDocFailure(f)
	if fail.Failure != f {
		t.Fatalf("NewDocFailure: Failure = %v, want %v", fail.Failure, f)
	}
	if fail.Doc != nil {
		t.Fatalf("NewDocFailure: Doc should be nil, got %v", fail.Doc)
	}

	// Nil guards: both panic to catch programming errors early.
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("NewDocResult(nil) should panic")
		}
	}()
	NewDocResult(nil)
}

func TestDocumentOrFailure_NewDocFailure_PanicsOnNil(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("NewDocFailure(nil) should panic")
		}
	}()
	NewDocFailure(nil)
}

// ---------------------------------------------------------------------
// 2. ConnectorFailure preserves the Cause chain
// ---------------------------------------------------------------------

func TestConnectorFailure_PreservesCauseChain(t *testing.T) {
	original := errors.New("upstream 429")
	wrapped := NewDocumentFailure("doc-1", "https://example.com/1", "rate limited", original)

	if !errors.Is(wrapped.Cause, original) {
		t.Fatalf("errors.Is did not unwrap to original: %v", wrapped.Cause)
	}
	if wrapped.FailedDocument == nil || wrapped.FailedDocument.DocID != "doc-1" {
		t.Fatalf("FailedDocument not populated correctly: %+v", wrapped.FailedDocument)
	}
	if wrapped.FailedEntity != nil {
		t.Fatalf("FailedEntity must be nil for NewDocumentFailure")
	}
	if wrapped.FailureMessage != "rate limited" {
		t.Fatalf("FailureMessage = %q, want %q", wrapped.FailureMessage, "rate limited")
	}

	// Entity variant: same invariants + time range preserved.
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	ent := NewEntityFailure("repo-a", "poll-window missed", &start, &end, original)
	if ent.FailedDocument != nil {
		t.Fatalf("FailedDocument must be nil for NewEntityFailure")
	}
	if ent.FailedEntity == nil || ent.FailedEntity.EntityID != "repo-a" {
		t.Fatalf("FailedEntity not populated: %+v", ent.FailedEntity)
	}
	if !ent.FailedEntity.MissedTimeRangeStart.Equal(start) || !ent.FailedEntity.MissedTimeRangeEnd.Equal(end) {
		t.Fatalf("time range not preserved: %+v", ent.FailedEntity)
	}
	if !errors.Is(ent.Cause, original) {
		t.Fatalf("entity failure errors.Is did not unwrap: %v", ent.Cause)
	}
}

// ---------------------------------------------------------------------
// 3. Registry Register + Lookup happy path
// ---------------------------------------------------------------------

func TestRegistry_RegisterAndLookup(t *testing.T) {
	t.Cleanup(resetRegistryForTest)
	resetRegistryForTest()

	factoryA := func(_ Source, _ *nango.Client) (Connector, error) {
		return &stubConnector{kind: "test-a"}, nil
	}
	Register("test-a", factoryA)

	got, err := Lookup("test-a")
	if err != nil {
		t.Fatalf("Lookup(test-a): unexpected error: %v", err)
	}

	// Prove it's the same factory: invoke it and check the kind.
	c, err := got(&stubSource{kind: "test-a"}, nil)
	if err != nil {
		t.Fatalf("factory invocation failed: %v", err)
	}
	if c.Kind() != "test-a" {
		t.Fatalf("factory returned connector with kind %q, want %q", c.Kind(), "test-a")
	}
}

// ---------------------------------------------------------------------
// 4. Duplicate registration panics
// ---------------------------------------------------------------------

func TestRegistry_DuplicateKindPanics(t *testing.T) {
	t.Cleanup(resetRegistryForTest)
	resetRegistryForTest()

	factory := func(_ Source, _ *nango.Client) (Connector, error) {
		return &stubConnector{kind: "dup"}, nil
	}
	Register("dup", factory)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("duplicate Register should panic")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value should be string, got %T: %v", r, r)
		}
		// Message should name the offending kind so ops can grep logs.
		if !contains(msg, "dup") {
			t.Fatalf("panic message %q should mention the kind", msg)
		}
	}()
	Register("dup", factory)
}

// ---------------------------------------------------------------------
// 5. Unknown kind returns ErrUnknownKind
// ---------------------------------------------------------------------

func TestRegistry_UnknownKind_ReturnsErrUnknownKind(t *testing.T) {
	t.Cleanup(resetRegistryForTest)
	resetRegistryForTest()

	_, err := Lookup("never-registered")
	if err == nil {
		t.Fatalf("Lookup on empty registry must return error")
	}
	if !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("err = %v, want errors.Is(err, ErrUnknownKind) = true", err)
	}
}

// ---------------------------------------------------------------------
// 6. RegisteredKinds is sorted deterministically
// ---------------------------------------------------------------------

func TestRegistry_RegisteredKinds_SortedDeterministic(t *testing.T) {
	t.Cleanup(resetRegistryForTest)
	resetRegistryForTest()

	// Register intentionally out of alphabetical order.
	for _, k := range []string{"notion", "github", "slack", "confluence"} {
		kind := k // capture
		Register(kind, func(_ Source, _ *nango.Client) (Connector, error) {
			return &stubConnector{kind: kind}, nil
		})
	}

	got := RegisteredKinds()
	want := []string{"confluence", "github", "notion", "slack"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RegisteredKinds() = %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------
// 7. Empty Section is legal by contract
// ---------------------------------------------------------------------

