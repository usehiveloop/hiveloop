package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// EstimateTotal sums issue + PR counts across every configured repo.
// Counts come from the rel="last" page number on a per_page=1 request,
// which is the GitHub idiom for "give me a total without paginating
// the result set." Issues with PullRequest != nil are still counted as
// PRs by GitHub on the issues endpoint, so we explicitly query /pulls
// and /issues separately and rely on the same dedup that the fetch
// stages already do.
func (c *GithubConnector) EstimateTotal(ctx context.Context, src interfaces.Source) (int, error) {
	cfg, err := LoadConfig(src.Config())
	if err != nil {
		return 0, err
	}
	if len(cfg.Repositories) == 0 {
		return 0, nil
	}

	total := 0
	for _, repo := range cfg.Repositories {
		fullName := cfg.RepoOwner + "/" + repo
		if cfg.IncludePRs {
			n, err := c.client.countViaLink(ctx, "/repos/"+fullName+"/pulls", cfg.StateFilter)
			if err != nil {
				return 0, fmt.Errorf("github estimate: pulls %s: %w", fullName, err)
			}
			total += n
		}
		if cfg.IncludeIssues {
			n, err := c.client.countViaLink(ctx, "/repos/"+fullName+"/issues", cfg.StateFilter)
			if err != nil {
				return 0, fmt.Errorf("github estimate: issues %s: %w", fullName, err)
			}
			// /issues includes PRs; subtracting the PR count here would
			// be cleaner, but GitHub doesn't separate the two cheaply
			// and the fetch stages already dedup PR-shaped issues.
			total += n
		}
	}
	return total, nil
}

// countViaLink probes a list endpoint with per_page=1 and reads the
// rel="last" page number from the Link header. GitHub returns no Link
// header for empty or single-item lists; in that case the count is the
// length of the returned page (0 or 1).
func (c *Client) countViaLink(ctx context.Context, path, state string) (int, error) {
	q := url.Values{}
	q.Set("per_page", "1")
	q.Set("page", "1")
	if state != "" {
		q.Set("state", state)
	}

	var hdr http.Header
	var first []map[string]any
	err := withRateLimitRetry(ctx, func() error {
		got, h, err := getJSON[[]map[string]any](ctx, c.p, http.MethodGet, path, q)
		if err != nil {
			return err
		}
		first = got
		hdr = h
		return nil
	})
	if err != nil {
		return 0, err
	}
	if last, ok := lastPageNumber(hdr); ok {
		return last, nil
	}
	return len(first), nil
}

