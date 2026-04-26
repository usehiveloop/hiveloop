package github

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
)

func makePermFixture(t *testing.T, visibility string) (*GithubConnector, *fakeProxy) {
	t.Helper()
	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "all", IncludePRs: true, IncludeIssues: true,
	}
	fp := newFakeProxy()
	fp.addDefault("GET", "/repos/"+repoFullName, mustMarshal(t, makeRepo(visibility)))
	c := NewConnector(cfg, fp)
	return c, fp
}

func TestPermSync_PublicRepoIsPublic(t *testing.T) {
	c, fp := makePermFixture(t, "public")

	// Need an empty PR + Issue page for SyncDocPermissions to terminate.
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 1, []byte(`[]`), 0)
	fp.addPage("GET", "/repos/"+repoFullName+"/issues", 1, []byte(`[]`), 0)

	src := &fixtureSource{cfg: json.RawMessage(`{"repo_owner":"acme"}`)}

	// Per-doc ACL: public repo means IsPublic=true on every doc — so
	// "no PRs/issues" still produces no failures.
	docCh, err := c.SyncDocPermissions(context.Background(), src)
	if err != nil {
		t.Fatalf("SyncDocPermissions: %v", err)
	}
	for ev := range docCh {
		if ev.Failure != nil {
			t.Fatalf("unexpected failure: %v", ev.Failure)
		}
	}

	// Group enumeration: public repo emits zero groups.
	groupCh, err := c.SyncExternalGroups(context.Background(), src)
	if err != nil {
		t.Fatalf("SyncExternalGroups: %v", err)
	}
	groups, fails := drainGroups(t, groupCh)
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %v", fails)
	}
	if len(groups) != 0 {
		t.Fatalf("public repo should emit zero groups; got %d", len(groups))
	}
}

func TestPermSync_PrivateRepoEnumeratesGroups(t *testing.T) {
	c, fp := makePermFixture(t, "private")

	// 3 direct collaborators, 1 outside, 2 teams.
	directs := []GithubUser{
		{ID: 11, Login: "alice", Email: "alice@example.com"},
		{ID: 12, Login: "bob", Email: "bob@example.com"},
		{ID: 13, Login: "carol"},
	}
	outsides := []GithubUser{{ID: 14, Login: "dave", Email: "dave@example.com"}}
	teams := []GithubTeam{
		{ID: 31, Slug: "backend", Name: "Backend"},
		{ID: 32, Slug: "platform", Name: "Platform"},
	}
	teamMembers := []GithubUser{{ID: 21, Login: "eve", Email: "eve@example.com"}}

	fp.addPage("GET", "/repos/"+repoFullName+"/collaborators", 1, mustMarshal(t, directs), 0)
	// The connector calls /collaborators twice with different affiliation
	// values; both share the same path key in the fake. Override the
	// second call with the outside list by re-registering — works
	// because we drain in order.
	// Simpler: register both via a tiny dispatcher map keyed on
	// affiliation. The fakeProxy doesn't key on query; for this test we
	// inline a lightweight middleware that picks the right slice based
	// on the current call counter.
	calls := 0
	fp.addDefault("GET", "/repos/"+repoFullName+"/collaborators",
		mustMarshal(t, directs)) // initial registration; overridden below.
	fp.handleCollaborators = func(affiliation string) []byte {
		calls++
		if affiliation == "direct" {
			return mustMarshal(t, directs)
		}
		return mustMarshal(t, outsides)
	}

	fp.addPage("GET", "/repos/"+repoFullName+"/teams", 1, mustMarshal(t, teams), 0)
	fp.addPage("GET", "/teams/31/members", 1, mustMarshal(t, teamMembers), 0)
	fp.addPage("GET", "/teams/32/members", 1, mustMarshal(t, teamMembers), 0)

	src := &fixtureSource{cfg: json.RawMessage(`{"repo_owner":"acme"}`)}
	groupCh, err := c.SyncExternalGroups(context.Background(), src)
	if err != nil {
		t.Fatalf("SyncExternalGroups: %v", err)
	}
	groups, fails := drainGroups(t, groupCh)
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %+v", fails)
	}

	// Private branch emits 4 groups: collaborators, outside-collaborators,
	// + 2 teams. (utils.py:249-277)
	if len(groups) != 4 {
		t.Fatalf("expected 4 private-repo groups, got %d (%+v)", len(groups), groups)
	}
	ids := make([]string, 0, 4)
	for _, g := range groups {
		ids = append(ids, g.GroupID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if id == "" || id[:len("external_group:")] != "external_group:" {
			t.Fatalf("group id missing prefix: %q", id)
		}
	}
}

func TestPermSync_InternalRepoBindsOrgGroup(t *testing.T) {
	c, fp := makePermFixture(t, "internal")

	members := []GithubMembership{
		{ID: 41, Login: "alice", Email: "alice@example.com"},
		{ID: 42, Login: "bob", Email: "bob@example.com"},
	}
	fp.addPage("GET", "/orgs/acme/members", 1, mustMarshal(t, members), 0)

	src := &fixtureSource{cfg: json.RawMessage(`{"repo_owner":"acme"}`)}
	groupCh, err := c.SyncExternalGroups(context.Background(), src)
	if err != nil {
		t.Fatalf("SyncExternalGroups: %v", err)
	}
	groups, fails := drainGroups(t, groupCh)
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %v", fails)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 org group, got %d (%+v)", len(groups), groups)
	}
	got := groups[0]
	if got.GroupID == "" {
		t.Fatal("org group ID is empty")
	}
	if len(got.MemberEmails) != 2 {
		t.Fatalf("expected 2 members, got %d", len(got.MemberEmails))
	}
}
