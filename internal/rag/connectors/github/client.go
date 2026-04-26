// Typed client over proxyClient.
//
// Each method is a few-line wrapper: build the path + query, call
// getJSON[T] inside withRateLimitRetry, parse the next-page header.
// No method touches a token; the proxyClient does that on its side.
//
// Onyx analog: the PyGithub `Github` instance the connector holds at
// connector.py:457 (`self.github_client = Github(...)`). PyGithub hides
// pagination + rate-limit + auth; we expose them at the boundary.
package github

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// pageSize matches Onyx's per_page=100 — the GitHub API maximum on
// list endpoints. Larger pages = fewer round-trips through Nango.
const pageSize = 100

// Client is the typed surface the rest of the connector uses. Holds the
// proxyClient + bound providerConfigKey/connectionID via newNangoProxy.
type Client struct {
	p proxyClient
}

// newClient builds the production client. Splits the constructor from
// the connector struct so tests can inject a fake proxyClient without
// going through nango.Client.
func newClient(p proxyClient) *Client {
	return &Client{p: p}
}

// listPullRequestsPage fetches one page of /repos/{owner}/{repo}/pulls
// in descending updated_at order (matches Onyx's `sort=updated&direction=desc`
// at connector.py:611). Returns the slice + the next page number (0 if
// there is none).
func (c *Client) listPullRequestsPage(
	ctx context.Context, fullName, state string, page int,
) ([]GithubPR, int, error) {
	q := url.Values{}
	q.Set("state", state)
	q.Set("sort", "updated")
	q.Set("direction", "desc")
	q.Set("per_page", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))

	var prs []GithubPR
	var hdr http.Header
	err := withRateLimitRetry(ctx, func() error {
		got, h, err := getJSON[[]GithubPR](ctx, c.p, http.MethodGet,
			"/repos/"+fullName+"/pulls", q)
		if err != nil {
			return err
		}
		prs = got
		hdr = h
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	next, _ := nextPageNumber(hdr)
	return prs, next, nil
}

// listIssuesPage fetches one page of /repos/{owner}/{repo}/issues. Note
// that GitHub returns PRs as issues; the caller filters out entries
// with PullRequest != nil (matches connector.py:710).
func (c *Client) listIssuesPage(
	ctx context.Context, fullName, state string, page int,
) ([]GithubIssue, int, error) {
	q := url.Values{}
	q.Set("state", state)
	q.Set("sort", "updated")
	q.Set("direction", "desc")
	q.Set("per_page", strconv.Itoa(pageSize))
	q.Set("page", strconv.Itoa(page))

	var issues []GithubIssue
	var hdr http.Header
	err := withRateLimitRetry(ctx, func() error {
		got, h, err := getJSON[[]GithubIssue](ctx, c.p, http.MethodGet,
			"/repos/"+fullName+"/issues", q)
		if err != nil {
			return err
		}
		issues = got
		hdr = h
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	next, _ := nextPageNumber(hdr)
	return issues, next, nil
}

// getRepo fetches /repos/{owner}/{repo} for visibility + ID + owner-id
// derivation (used by perm-sync to translate visibility into ACL).
func (c *Client) getRepo(ctx context.Context, fullName string) (GithubRepo, error) {
	var repo GithubRepo
	err := withRateLimitRetry(ctx, func() error {
		got, _, err := getJSON[GithubRepo](ctx, c.p, http.MethodGet,
			"/repos/"+fullName, nil)
		if err != nil {
			return err
		}
		repo = got
		return nil
	})
	return repo, err
}

// listCollaborators fetches /repos/{owner}/{repo}/collaborators with
// affiliation filter. Affiliation defaults to "all" (direct + outside);
// utils.py:96-128 splits direct vs outside via separate filters, which
// we mirror by exposing the param.
func (c *Client) listCollaborators(
	ctx context.Context, fullName, affiliation string,
) ([]GithubUser, error) {
	return listAllPaginated[GithubUser](ctx, c, "/repos/"+fullName+"/collaborators",
		url.Values{"affiliation": []string{affiliation}})
}

// listTeams fetches /repos/{owner}/{repo}/teams.
func (c *Client) listTeams(ctx context.Context, fullName string) ([]GithubTeam, error) {
	return listAllPaginated[GithubTeam](ctx, c, "/repos/"+fullName+"/teams", nil)
}

// listOrgMembers fetches /orgs/{org}/members. Used for the
// internal-visibility ACL branch (everyone in the org sees the doc).
func (c *Client) listOrgMembers(ctx context.Context, org string) ([]GithubMembership, error) {
	return listAllPaginated[GithubMembership](ctx, c, "/orgs/"+org+"/members", nil)
}

// listTeamMembers fetches /teams/{teamID}/members. Used for the private-
// repo group enumeration (team → member emails).
func (c *Client) listTeamMembers(ctx context.Context, teamID int64) ([]GithubUser, error) {
	return listAllPaginated[GithubUser](ctx, c,
		"/teams/"+strconv.FormatInt(teamID, 10)+"/members", nil)
}

// listAllPaginated walks every page until exhaustion, accumulating into
// a single slice. Used for endpoints where the result set is small
// enough to materialise in memory (collaborators, teams, org members).
//
// PR/Issue lists deliberately don't use this — they need streaming +
// per-page checkpointing, handled by listPullRequestsPage /
// listIssuesPage in the fetch loops.
func listAllPaginated[T any](
	ctx context.Context, c *Client, path string, q url.Values,
) ([]T, error) {
	if q == nil {
		q = url.Values{}
	}
	q.Set("per_page", strconv.Itoa(pageSize))

	var out []T
	page := 1
	for {
		q.Set("page", strconv.Itoa(page))
		var batch []T
		var hdr http.Header
		err := withRateLimitRetry(ctx, func() error {
			got, h, err := getJSON[[]T](ctx, c.p, http.MethodGet, path, q)
			if err != nil {
				return err
			}
			batch = got
			hdr = h
			return nil
		})
		if err != nil {
			return nil, err
		}
		out = append(out, batch...)
		next, ok := nextPageNumber(hdr)
		if !ok {
			return out, nil
		}
		page = next
	}
}
