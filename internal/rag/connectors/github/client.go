package github

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// pageSize is the GitHub API maximum on list endpoints.
const pageSize = 100

type Client struct {
	p proxyClient
}

func newClient(p proxyClient) *Client {
	return &Client{p: p}
}

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

// listIssuesPage: GitHub returns PRs as issues; the caller filters
// out entries with PullRequest != nil.
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

func (c *Client) listCollaborators(
	ctx context.Context, fullName, affiliation string,
) ([]GithubUser, error) {
	return listAllPaginated[GithubUser](ctx, c, "/repos/"+fullName+"/collaborators",
		url.Values{"affiliation": []string{affiliation}})
}

func (c *Client) listTeams(ctx context.Context, fullName string) ([]GithubTeam, error) {
	return listAllPaginated[GithubTeam](ctx, c, "/repos/"+fullName+"/teams", nil)
}

func (c *Client) listOrgMembers(ctx context.Context, org string) ([]GithubMembership, error) {
	return listAllPaginated[GithubMembership](ctx, c, "/orgs/"+org+"/members", nil)
}

func (c *Client) listTeamMembers(ctx context.Context, teamID int64) ([]GithubUser, error) {
	return listAllPaginated[GithubUser](ctx, c,
		"/teams/"+strconv.FormatInt(teamID, 10)+"/members", nil)
}

// listAllPaginated materialises the full result in memory; PR/Issue
// lists must use streaming + per-page checkpointing instead.
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
