package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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
		if cfg.IncludePRs {
			n, err := c.client.searchCount(ctx, fullName, "pr", cfg.StateFilter)
			if err != nil {
				return 0, fmt.Errorf("github estimate: prs %s: %w", fullName, err)
			}
			total += n
		}
		if cfg.IncludeIssues {
			n, err := c.client.searchCount(ctx, fullName, "issue", cfg.StateFilter)
			if err != nil {
				return 0, fmt.Errorf("github estimate: issues %s: %w", fullName, err)
			}
			total += n
		}
	}
	return total, nil
}

type searchIssuesResponse struct {
	TotalCount int `json:"total_count"`
}

func (c *Client) searchCount(ctx context.Context, fullName, kind, state string) (int, error) {
	parts := []string{"repo:" + fullName, "is:" + kind}
	if state != "" && state != "all" {
		parts = append(parts, "state:"+state)
	}
	q := url.Values{}
	q.Set("q", strings.Join(parts, " "))
	q.Set("per_page", "1")

	var resp searchIssuesResponse
	err := withRateLimitRetry(ctx, func() error {
		got, _, err := getJSON[searchIssuesResponse](ctx, c.p, http.MethodGet, "/search/issues", q)
		if err != nil {
			return err
		}
		resp = got
		return nil
	})
	if err != nil {
		return 0, err
	}
	return resp.TotalCount, nil
}
