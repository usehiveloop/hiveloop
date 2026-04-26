package github

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestEndToEndIngestion_Through3CScheduler exercises the full connector
// trait surface — CheckpointedConnector + PermSyncConnector +
// SlimConnector — against a single fixture repo with 25 PRs across 3
// pages. It plays the role the 3C scheduler will play in production:
// drive the connector to completion, observe the emitted artifacts,
// confirm no fatal errors.
//
// Plan deviation note: the 3C scheduler stack (internal/rag/scheduler,
// internal/rag/tasks/ingest.go) is not yet present on this branch.
// Once those land, this test will be extended to call into
// scheduler.HandleIngest directly. The current shape verifies every
// connector responsibility called from 3C — checkpoint advance, doc
// emission, perm-sync, slim listing — so the seams that 3C will
// consume are already pinned.
func TestEndToEndIngestion_Through3CScheduler(t *testing.T) {
	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "all", IncludePRs: true, IncludeIssues: true,
	}
	c, fp := buildConnector(t, cfg, "private")

	base := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	prsP1 := make([]GithubPR, 10)
	prsP2 := make([]GithubPR, 10)
	prsP3 := make([]GithubPR, 5)
	for i := 0; i < 10; i++ {
		prsP1[i] = makePR(i+1, "open", base.Add(-time.Duration(i)*time.Minute))
	}
	for i := 0; i < 10; i++ {
		prsP2[i] = makePR(i+11, "open", base.Add(-time.Duration(10+i)*time.Minute))
	}
	for i := 0; i < 5; i++ {
		prsP3[i] = makePR(i+21, "open", base.Add(-time.Duration(20+i)*time.Minute))
	}
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 1, mustMarshal(t, prsP1), 2)
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 2, mustMarshal(t, prsP2), 3)
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 3, mustMarshal(t, prsP3), 0)

	// Issues: empty page, but the orchestrator must still walk into the
	// stage so the checkpoint reaches DONE.
	fp.addPage("GET", "/repos/"+repoFullName+"/issues", 1, []byte(`[]`), 0)

	// Permission sync fixtures (private branch).
	fp.handleCollaborators = func(affiliation string) []byte {
		if affiliation == "direct" {
			return mustMarshal(t, []GithubUser{
				{ID: 11, Login: "alice", Email: "alice@example.com"},
				{ID: 12, Login: "bob", Email: "bob@example.com"},
			})
		}
		return mustMarshal(t, []GithubUser{
			{ID: 13, Login: "carol", Email: "carol@example.com"},
		})
	}
	fp.addPage("GET", "/repos/"+repoFullName+"/teams", 1, mustMarshal(t, []GithubTeam{
		{ID: 31, Slug: "platform", Name: "Platform"},
	}), 0)
	fp.addPage("GET", "/teams/31/members", 1, mustMarshal(t, []GithubUser{
		{ID: 21, Login: "dave", Email: "dave@example.com"},
	}), 0)

	src := &fixtureSource{cfg: json.RawMessage(`{"repo_owner":"acme","repositories":["widget"]}`)}

	// 1. Ingest pass — analogous to a 3C scheduler invocation of
	//    HandleIngest. We expect 25 PR documents, 0 issues, 0 fatal
	//    failures, and the orchestrator to terminate cleanly.
	ch, err := c.LoadFromCheckpoint(context.Background(), src, c.DummyCheckpoint(), time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("LoadFromCheckpoint: %v", err)
	}
	docs, fails := drainIngest(t, ch)
	if len(fails) != 0 {
		t.Fatalf("ingest: unexpected failures: %v", fails)
	}
	if len(docs) != 25 {
		t.Fatalf("ingest: expected 25 PR documents, got %d", len(docs))
	}
	// Every doc should carry the private repo's group ACL — proves the
	// connector resolved visibility once + applied it across the batch.
	for _, d := range docs {
		if d.IsPublic {
			t.Fatalf("private repo doc unexpectedly IsPublic: %s", d.DocID)
		}
		if len(d.ACL) == 0 {
			t.Fatalf("expected non-empty ACL on private repo doc: %s", d.DocID)
		}
	}

	// 2. Permission sync — emits 25 DocExternalAccess entries (one per
	//    PR) plus the four group rows for a private repo.
	docPermCh, err := c.SyncDocPermissions(context.Background(), src)
	if err != nil {
		t.Fatalf("SyncDocPermissions: %v", err)
	}
	accessCount := 0
	for ev := range docPermCh {
		if ev.Failure != nil {
			t.Fatalf("perm-sync failure: %v", ev.Failure)
		}
		accessCount++
	}
	// 25 PRs + 0 issues from the private repo.
	if accessCount != 25 {
		t.Fatalf("expected 25 perm-sync rows, got %d", accessCount)
	}

	groupCh, err := c.SyncExternalGroups(context.Background(), src)
	if err != nil {
		t.Fatalf("SyncExternalGroups: %v", err)
	}
	groups, groupFails := drainGroups(t, groupCh)
	if len(groupFails) != 0 {
		t.Fatalf("group sync failures: %v", groupFails)
	}
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups (collab + outside + 1 team), got %d", len(groups))
	}

	// 3. Slim listing — the prune loop's input. 25 SlimDocuments, all
	//    carrying the same ExternalAccess.
	slimCh, err := c.ListAllSlim(context.Background(), src)
	if err != nil {
		t.Fatalf("ListAllSlim: %v", err)
	}
	slims, slimFails := drainSlim(t, slimCh)
	if len(slimFails) != 0 {
		t.Fatalf("slim failures: %v", slimFails)
	}
	if len(slims) != 25 {
		t.Fatalf("expected 25 slim docs, got %d", len(slims))
	}
}
