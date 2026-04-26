package github

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestSlimDocs_ReturnsIDsOnly(t *testing.T) {
	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "all", IncludePRs: true, IncludeIssues: false,
	}
	c, fp := buildConnector(t, cfg, "public")

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

	src := &fixtureSource{cfg: json.RawMessage(`{"repo_owner":"acme","repositories":["widget"]}`)}
	ch, err := c.ListAllSlim(context.Background(), src)
	if err != nil {
		t.Fatalf("ListAllSlim: %v", err)
	}
	slims, fails := drainSlim(t, ch)
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %v", fails)
	}
	if len(slims) != 25 {
		t.Fatalf("expected 25 slim docs, got %d", len(slims))
	}
	for _, s := range slims {
		if s.DocID == "" {
			t.Fatalf("slim doc with empty DocID: %+v", s)
		}
		if s.ExternalAccess == nil {
			t.Fatalf("expected ExternalAccess on slim, got nil")
		}
	}
}
