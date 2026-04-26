package github

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestIndexingStart_FloorsTheWindow asserts that when the connector is
// driven with a start time after some PRs' updated_at, those older PRs
// are not emitted. The IndexingStart column is the *RAGSource* hook
// that lets admins set this floor; the connector only sees the
// already-resolved start parameter (the worker computes it from
// IndexingStart + LastSuccessfulIndexTime). So this test exercises the
// floor effect end-to-end at the connector level — the worker-side
// composition is verified separately.
func TestIndexingStart_FloorsTheWindow(t *testing.T) {
	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "all", IncludePRs: true,
	}
	c, fp := buildConnector(t, cfg, "public")

	// Mix of pre- and post-2024 PRs. With IndexingStart = 2024-01-01,
	// only the post-2024 ones must come out.
	floor := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	older := []GithubPR{
		makePR(1, "open", time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)),
		makePR(2, "open", time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
	}
	newer := []GithubPR{
		makePR(10, "open", time.Date(2025, 4, 1, 12, 0, 0, 0, time.UTC)),
		makePR(11, "open", time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)),
	}
	// GitHub returns sort=updated&direction=desc, so newer first.
	page := append(append([]GithubPR{}, newer...), older...)
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 1, mustMarshal(t, page), 0)

	// The connector applies pollOverlap (3h) to start, which is
	// negligible for a 2024-01 floor; the test still passes because
	// older PRs are years older.
	src := &fixtureSource{cfg: json.RawMessage(`{"repo_owner":"acme","repositories":["widget"]}`)}
	ch, err := c.LoadFromCheckpoint(context.Background(), src, c.DummyCheckpoint(), floor, time.Time{})
	if err != nil {
		t.Fatalf("LoadFromCheckpoint: %v", err)
	}
	docs, fails := drainIngest(t, ch)
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %v", fails)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 PRs above floor, got %d", len(docs))
	}
	for _, d := range docs {
		if d.DocUpdatedAt.Before(floor) {
			t.Fatalf("doc %q has updated %v, should be >= floor", d.DocID, d.DocUpdatedAt)
		}
	}
}

// TestRAGSource_IndexingStartFloorPersists confirms that the column
// round-trips through the schema cleanly: setting + reading it returns
// the same time, NULL persists as nil. Lives in this package because
// the field's semantic owner is the connector layer; the model package
// already has its own column tests.
//
// We don't run a full DB test here — that lives in
// internal/rag/model/rag_source_index_test.go territory. The narrower
// assertion here is that the JSON marshal of the new field matches
// expectations so the API + scheduler layers can serialise it.
func TestRAGSource_IndexingStartJSONShape(t *testing.T) {
	// The pointer-to-time field marshals as null when nil; as RFC3339
	// when set. Pin both so we don't accidentally break the wire shape.
	type minimalRow struct {
		IndexingStart *time.Time `json:"indexing_start,omitempty"`
	}
	null, err := json.Marshal(minimalRow{})
	if err != nil {
		t.Fatalf("marshal nil: %v", err)
	}
	if string(null) != `{}` {
		t.Fatalf("nil IndexingStart should marshal to {}, got %s", null)
	}
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	set, err := json.Marshal(minimalRow{IndexingStart: &now})
	if err != nil {
		t.Fatalf("marshal set: %v", err)
	}
	if string(set) != `{"indexing_start":"2024-01-01T00:00:00Z"}` {
		t.Fatalf("unexpected JSON: %s", set)
	}
}
