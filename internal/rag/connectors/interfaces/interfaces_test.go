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

func TestSectionEmpty_AllowedByContract(t *testing.T) {
	// The contract: Section{Text: ""} is a valid value; the chunker
	// (Phase 2E) is responsible for skipping empty sections. This test
	// pins the contract by exercising construction + round-trip without
	// any validation gate firing.
	s := Section{Text: ""}
	if s.Text != "" {
		t.Fatalf("Section.Text = %q, want empty", s.Text)
	}

	// A Document built from empty sections round-trips cleanly.
	doc := Document{
		DocID:      "d",
		SemanticID: "d",
		Sections:   []Section{s, {Text: "hello"}, {}},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Document
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back.Sections) != 3 {
		t.Fatalf("round-trip lost sections: got %d, want 3", len(back.Sections))
	}
}

// ---------------------------------------------------------------------
// 8. Document JSON round-trip preserves every field
// ---------------------------------------------------------------------

func TestDocument_JSONRoundtrip(t *testing.T) {
	updated := time.Date(2026, 4, 22, 12, 30, 45, 0, time.UTC)
	orig := Document{
		DocID:      "gh-pr-42",
		SemanticID: "Fix the flaky test",
		Link:       "https://github.com/acme/foo/pull/42",
		Sections: []Section{
			{Text: "body", Link: "https://github.com/acme/foo/pull/42", Title: "PR body"},
			{Text: "comment one", Link: "https://github.com/acme/foo/pull/42#c1"},
		},
		ACL:             []string{"user_email:alice@example.com", "external_group:github_acme_backend"},
		IsPublic:        false,
		DocUpdatedAt:    &updated,
		Metadata:        map[string]string{"state": "closed", "repo": "acme/foo"},
		PrimaryOwners:   []string{"alice@example.com"},
		SecondaryOwners: []string{"bob@example.com", "carol@example.com"},
	}

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back Document
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// DocUpdatedAt: pointer comparison is wrong; compare Time values.
	if back.DocUpdatedAt == nil || !back.DocUpdatedAt.Equal(*orig.DocUpdatedAt) {
		t.Fatalf("DocUpdatedAt lost in round-trip: %v vs %v", back.DocUpdatedAt, orig.DocUpdatedAt)
	}
	// Replace pointer fields for the structural compare.
	back.DocUpdatedAt = orig.DocUpdatedAt

	if !reflect.DeepEqual(orig, back) {
		t.Fatalf("round-trip not equal:\norig=%+v\nback=%+v\nraw=%s", orig, back, string(raw))
	}
}

// ---------------------------------------------------------------------
// 9. ExternalAccess: IsPublic with empty user scope is legal
// ---------------------------------------------------------------------

func TestExternalAccess_PublicDocAllowsEmptyUserScope(t *testing.T) {
	ea := ExternalAccess{IsPublic: true}
	if !ea.IsPublic {
		t.Fatalf("IsPublic should be true")
	}
	if len(ea.ExternalUserEmails) != 0 {
		t.Fatalf("expected empty emails, got %v", ea.ExternalUserEmails)
	}
	if len(ea.ExternalUserGroupIDs) != 0 {
		t.Fatalf("expected empty group ids, got %v", ea.ExternalUserGroupIDs)
	}

	// Round-trip: public-empty-scope must survive JSON unchanged.
	raw, err := json.Marshal(ea)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back ExternalAccess
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !back.IsPublic {
		t.Fatalf("round-trip dropped IsPublic; raw=%s", string(raw))
	}
}

// ---------------------------------------------------------------------
// 10. DocExternalAccess round-trips
// ---------------------------------------------------------------------

func TestExternalAccess_DocExternalAccessLinksToDoc(t *testing.T) {
	dea := DocExternalAccess{
		DocID: "gh-pr-42",
		ExternalAccess: &ExternalAccess{
			ExternalUserEmails:   []string{"alice@example.com"},
			ExternalUserGroupIDs: []string{"external_group:github_acme_backend"},
			IsPublic:             false,
		},
	}

	raw, err := json.Marshal(dea)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back DocExternalAccess
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if back.DocID != dea.DocID {
		t.Fatalf("DocID lost: %q vs %q", back.DocID, dea.DocID)
	}
	if back.ExternalAccess == nil {
		t.Fatalf("ExternalAccess pointer lost")
	}
	if !reflect.DeepEqual(back.ExternalAccess, dea.ExternalAccess) {
		t.Fatalf("ExternalAccess mismatch:\norig=%+v\nback=%+v", dea.ExternalAccess, back.ExternalAccess)
	}
}

// ---------------------------------------------------------------------
// 11. Checkpoint marker interface composes with generics
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
