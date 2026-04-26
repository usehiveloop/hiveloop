// SlimDocs: cheap doc-ID-only listing for the prune diff.
//
// The scheduler diffs SlimDocument IDs against the locally-indexed set
// to detect source-side deletions. We don't need bodies or metadata
// here — just the IDs and (cheaply) the per-repo ExternalAccess so the
// prune side can short-circuit a perm-sync pass for unchanged docs.
//
// Onyx analog: connector.py:837-889 (`retrieve_all_slim_documents`).
package github

import (
	"context"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// ListAllSlim implements interfaces.SlimConnector. Walks each configured
// repo, lists every PR + Issue (state=all), emits a SlimDocument with
// the repo-level ExternalAccess. Same pagination as the ingest path,
// but no document conversion or section emission.
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

// streamRepoSlim drains every PR + Issue in a repo as SlimDocument.
// Reuses the listing endpoints (vs. building a separate slim API) so
// the prune diff sees the exact same doc-id alphabet as the ingest
// path.
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
