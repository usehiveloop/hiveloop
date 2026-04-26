package github

import (
	"context"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

func (c *GithubConnector) ListAllSlim(
	ctx context.Context, _ interfaces.Source,
) (<-chan interfaces.SlimDocOrFailure, error) {
	out := make(chan interfaces.SlimDocOrFailure, c.channelBuf)
	go func() {
		defer close(out)
		for _, full := range c.repoFullNames() {
			repo, err := c.client.getRepo(ctx, full)
			if err != nil {
				out <- interfaces.NewSlimFailure(entityFailure(full, "github: get repo for slim", err))
				continue
			}
			access := mapVisibility(repo)
			c.streamRepoSlim(ctx, full, access, out)
		}
	}()
	return out, nil
}

// Reuses the same listing endpoints as ingest so the prune diff sees
// the exact same doc-id alphabet.
func (c *GithubConnector) streamRepoSlim(
	ctx context.Context, fullName string, access *interfaces.ExternalAccess,
	out chan<- interfaces.SlimDocOrFailure,
) {
	if c.cfg.IncludePRs {
		page := 1
		for {
			prs, next, err := c.client.listPullRequestsPage(ctx, fullName, "all", page)
			if err != nil {
				out <- interfaces.NewSlimFailure(entityFailure(fullName, "github: list PRs slim", err))
				break
			}
			for _, pr := range prs {
				out <- interfaces.NewSlimResult(&interfaces.SlimDocument{
					DocID:          docIDForPR(fullName, pr),
					ExternalAccess: access,
				})
			}
			if next == 0 {
				break
			}
			page = next
		}
	}
	if c.cfg.IncludeIssues {
		page := 1
		for {
			issues, next, err := c.client.listIssuesPage(ctx, fullName, "all", page)
			if err != nil {
				out <- interfaces.NewSlimFailure(entityFailure(fullName, "github: list issues slim", err))
				break
			}
			for _, issue := range issues {
				if issue.PullRequest != nil {
					continue
				}
				out <- interfaces.NewSlimResult(&interfaces.SlimDocument{
					DocID:          docIDForIssue(fullName, issue),
					ExternalAccess: access,
				})
			}
			if next == 0 {
				break
			}
			page = next
		}
	}
}
