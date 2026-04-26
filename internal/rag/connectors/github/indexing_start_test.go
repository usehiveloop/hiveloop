package github

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestIndexingStart_FloorsTheWindow(t *testing.T) {
	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "all", IncludePRs: true,
	}
	c, fp := buildConnector(t, cfg, "public")

	floor := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	older := []GithubPR{
		makePR(1, "open", time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)),
		makePR(2, "open", time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)),
	}
	newer := []GithubPR{
		makePR(10, "open", time.Date(2025, 4, 1, 12, 0, 0, 0, time.UTC)),
		makePR(11, "open", time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)),
	}
	page := append(append([]GithubPR{}, newer...), older...)
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 1, mustMarshal(t, page), 0)

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

func TestRAGSource_IndexingStartJSONShape(t *testing.T) {
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
