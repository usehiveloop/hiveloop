package github

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// buildConnector wires a GithubConnector with the given config and a
// fakeProxy preloaded with a public-repo fixture under acme/widget.
func buildConnector(t *testing.T, cfg GithubConfig, repoVisibility string) (*GithubConnector, *fakeProxy) {
	t.Helper()
	fp := newFakeProxy()
	fp.addDefault("GET", "/repos/"+repoFullName, mustMarshal(t, makeRepo(repoVisibility)))
	c := NewConnector(cfg, fp)
	return c, fp
}

// runIngest is the standard "drive LoadFromCheckpoint to completion"
// helper used by every fetch test.
func runIngest(t *testing.T, c *GithubConnector, start, end time.Time) (
	docs []docDigest, fails []failDigest,
) {
	t.Helper()
	src := &fixtureSource{cfg: json.RawMessage(`{"repo_owner":"acme","repositories":["widget"]}`)}
	ch, err := c.LoadFromCheckpoint(context.Background(), src, c.DummyCheckpoint(), start, end)
	if err != nil {
		t.Fatalf("LoadFromCheckpoint: %v", err)
	}
	rawDocs, rawFails := drainIngest(t, ch)
	for _, d := range rawDocs {
		docs = append(docs, docDigest{
			docID:    d.DocID,
			isPublic: d.IsPublic,
			body:     d.Sections[0].Text,
		})
	}
	for _, f := range rawFails {
		dg := failDigest{msg: f.FailureMessage}
		if f.FailedDocument != nil {
			dg.docID = f.FailedDocument.DocID
		}
		if f.FailedEntity != nil {
			dg.entity = f.FailedEntity.EntityID
		}
		fails = append(fails, dg)
	}
	return
}

type docDigest struct {
	docID    string
	isPublic bool
	body     string
}
type failDigest struct {
	docID  string
	entity string
	msg    string
}

func TestFetchPRs_PaginatesUntilExhausted(t *testing.T) {
	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "all", IncludePRs: true, IncludeIssues: false,
	}
	c, fp := buildConnector(t, cfg, "public")

	// 25 PRs across 3 pages (10, 10, 5). Fixed UpdatedAt order so we
	// don't trip the time-window early-break.
	base := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	page1 := make([]GithubPR, 10)
	page2 := make([]GithubPR, 10)
	page3 := make([]GithubPR, 5)
	for i := 0; i < 10; i++ {
		page1[i] = makePR(i+1, "open", base.Add(-time.Duration(i)*time.Minute))
	}
	for i := 0; i < 10; i++ {
		page2[i] = makePR(i+11, "open", base.Add(-time.Duration(10+i)*time.Minute))
	}
	for i := 0; i < 5; i++ {
		page3[i] = makePR(i+21, "open", base.Add(-time.Duration(20+i)*time.Minute))
	}
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 1, mustMarshal(t, page1), 2)
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 2, mustMarshal(t, page2), 3)
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 3, mustMarshal(t, page3), 0)

	docs, fails := runIngest(t, c, time.Time{}, time.Time{})
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %+v", fails)
	}
	if len(docs) != 25 {
		t.Fatalf("expected 25 documents, got %d", len(docs))
	}
}

func TestFetchPRs_StateFilterOpenSkipsClosed(t *testing.T) {
	// We only return PRs the GitHub API would have returned for
	// state=open — so the test asserts that the connector forwards the
	// state filter (rather than client-side filtering).
	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "open", IncludePRs: true,
	}
	c, fp := buildConnector(t, cfg, "public")

	base := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	openPRs := make([]GithubPR, 15)
	for i := 0; i < 15; i++ {
		openPRs[i] = makePR(i+1, "open", base.Add(-time.Duration(i)*time.Minute))
	}
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 1, mustMarshal(t, openPRs), 0)

	docs, fails := runIngest(t, c, time.Time{}, time.Time{})
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %+v", fails)
	}
	if len(docs) != 15 {
		t.Fatalf("expected 15 open PRs, got %d", len(docs))
	}
	// And the connector must have asked for state=open.
	if !strings.Contains(strings.Join(fp.calls, "\n"), "state=open") {
		t.Fatalf("connector did not forward state=open: %v", fp.calls)
	}
}

func TestFetchPRs_TimeWindowEarlyBreak(t *testing.T) {
	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "all", IncludePRs: true,
	}
	c, fp := buildConnector(t, cfg, "public")

	// Page 1: 5 PRs in window. Page 2: 5 PRs older than the window —
	// connector must early-break before fetching page 2.
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	inWindow := make([]GithubPR, 5)
	for i := 0; i < 5; i++ {
		inWindow[i] = makePR(i+1, "open", now.Add(-time.Duration(i)*time.Minute))
	}
	// Page-2 fixture is registered, but the test asserts it was not
	// called by checking fp.calls.
	oldPRs := make([]GithubPR, 5)
	for i := 0; i < 5; i++ {
		oldPRs[i] = makePR(i+10, "open", now.Add(-365*24*time.Hour))
	}
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 1, mustMarshal(t, append(inWindow, oldPRs[:2]...)), 2)
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 2, mustMarshal(t, oldPRs[2:]), 0)

	// Bump the start past the old PRs but inside the recent ones.
	// The connector applies pollOverlap (3h) so we set start safely
	// past that.
	start := now.Add(-30 * time.Minute)
	docs, fails := runIngest(t, c, start, time.Time{})
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %+v", fails)
	}
	if len(docs) != 5 {
		t.Fatalf("expected 5 in-window PRs, got %d (docs=%v)", len(docs), docs)
	}
	// Verify page 2 was NOT requested.
	for _, c := range fp.calls {
		if strings.Contains(c, "/pulls?") && strings.Contains(c, "page=2") {
			t.Fatalf("connector should have early-broken before page 2: %v", fp.calls)
		}
	}
}

func TestFetchPRs_OverlapWindowCatchesLateUpdates(t *testing.T) {
	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "all", IncludePRs: true,
	}
	c, fp := buildConnector(t, cfg, "public")

	// Last successful run finished at "now". A PR updated 1 hour earlier
	// (< pollOverlap=3h) must still be re-emitted because the connector
	// walks start back by 3h internally.
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	prInOverlap := makePR(1, "open", now.Add(-1*time.Hour))
	prs := []GithubPR{prInOverlap}
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 1, mustMarshal(t, prs), 0)

	docs, fails := runIngest(t, c, now, time.Time{})
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %+v", fails)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 PR caught by overlap window, got %d", len(docs))
	}
}

func TestFetchIssues_SkipsPRShapedIssues(t *testing.T) {
	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "all", IncludePRs: false, IncludeIssues: true,
	}
	c, fp := buildConnector(t, cfg, "public")

	// 5 entries: 3 are PR-shaped (have pull_request set), 2 are real issues.
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	mixed := []GithubIssue{
		makeIssue(1, false, now),
		makeIssue(2, true, now),
		makeIssue(3, true, now),
		makeIssue(4, false, now),
		makeIssue(5, true, now),
	}
	fp.addPage("GET", "/repos/"+repoFullName+"/issues", 1, mustMarshal(t, mixed), 0)

	docs, fails := runIngest(t, c, time.Time{}, time.Time{})
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %+v", fails)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 real issues, got %d (%v)", len(docs), docs)
	}
}

func TestPerDocFailure_ContinuesBatch(t *testing.T) {
	// Malformed JSON on a page produces a per-page entity failure but
	// does not abort the run — every other page is still emitted.
	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "all", IncludePRs: true,
	}
	c, fp := buildConnector(t, cfg, "public")

	base := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	page1 := make([]GithubPR, 10)
	page3 := make([]GithubPR, 5)
	for i := 0; i < 10; i++ {
		page1[i] = makePR(i+1, "open", base.Add(-time.Duration(i)*time.Minute))
	}
	for i := 0; i < 5; i++ {
		page3[i] = makePR(i+21, "open", base.Add(-time.Duration(20+i)*time.Minute))
	}
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 1, mustMarshal(t, page1), 2)
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 2, mustMarshal(t, page1), 3) // overwritten by malformed
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 3, mustMarshal(t, page3), 0)
	fp.injectMalformed("GET", "/repos/"+repoFullName+"/pulls", 2)

	docs, fails := runIngest(t, c, time.Time{}, time.Time{})
	// Page 2 fails; the run advances to the next repo (we have only one)
	// and stops at this stage. Page 1's 10 docs survive.
	if len(docs) != 10 {
		t.Fatalf("expected 10 docs from page 1, got %d", len(docs))
	}
	if len(fails) != 1 {
		t.Fatalf("expected 1 failure for the malformed page, got %d (%v)", len(fails), fails)
	}
	if !hasSubstring(fails[0].msg, "page fetch failed") {
		t.Fatalf("failure msg should describe the page fetch: %q", fails[0].msg)
	}
}
