package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

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
		var prCount int
		if cfg.IncludePRs || cfg.IncludeIssues {
			n, err := c.client.countViaLink(ctx, "/repos/"+fullName+"/pulls", cfg.StateFilter)
			if err != nil {
				return 0, fmt.Errorf("github estimate: pulls %s: %w", fullName, err)
			}
			prCount = n
		}
		if cfg.IncludePRs {
			total += prCount
		}
		if cfg.IncludeIssues {
			// /issues returns issues + PRs; subtract PRs to get the
			// actual issue count the connector will emit (it filters
			// PR-shaped items out of the issues stream).
			n, err := c.client.countViaLink(ctx, "/repos/"+fullName+"/issues", cfg.StateFilter)
			if err != nil {
				return 0, fmt.Errorf("github estimate: issues %s: %w", fullName, err)
			}
			issuesOnly := n - prCount
			if issuesOnly < 0 {
				issuesOnly = 0
			}
			total += issuesOnly
		}
	}
	return total, nil
}

// countViaLink probes the list endpoint at per_page=1 and reads the
// rel="last" page number from the Link header — GitHub omits Link for
// empty/single-item lists, so fall back to len(body).
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

