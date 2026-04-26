// Issue-fetch loop. Same shape as fetch_prs.go; differs in two ways:
//
//  1. We filter out entries whose `pull_request` field is set — GitHub
//     returns PRs as issues on /repos/.../issues, and Onyx skips them
//     at connector.py:710 (`if issue.pull_request: continue`).
//  2. The Document mapping comes from issueToDocument, not prToDocument.
//
// Onyx analog: connector.py:694-754 (`_fetch_issues`).
package github

import (
	"context"
	"errors"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// fetchIssuesPage drains one page of /repos/{owner}/{repo}/issues. Same
// return semantics as fetchPRsPage.
func fetchIssuesPage(
	ctx context.Context,
	client *Client,
	fullName, state string,
	cp *GithubCheckpoint,
	start, end time.Time,
	access *interfaces.ExternalAccess,
	out chan<- interfaces.DocumentOrFailure,
) bool {
	issues, next, err := client.listIssuesPage(ctx, fullName, state, cp.CurrPage)
	if err != nil {
		if errors.Is(err, errRateLimited) {
			out <- interfaces.NewDocFailure(entityFailure(fullName,
				"github: issues page exhausted retries", err))
		} else {
			out <- interfaces.NewDocFailure(entityFailure(fullName,
				"github: issues page fetch failed", err))
		}
		return true
	}

	earlyBreak := false
	for i := range issues {
		issue := issues[i]
		// Drop PR-shaped issues: GitHub returns them on this endpoint
		// but they're already covered by /pulls (or excluded entirely
		// by IncludePRs=false).
		if issue.PullRequest != nil {
			continue
		}
		if !end.IsZero() && issue.UpdatedAt.After(end) {
			continue
		}
		if !start.IsZero() && issue.UpdatedAt.Before(start) {
			earlyBreak = true
			break
		}
		if cp.LastSeenUpdatedAt == nil || issue.UpdatedAt.After(*cp.LastSeenUpdatedAt) {
			t := issue.UpdatedAt
			cp.LastSeenUpdatedAt = &t
		}
		doc := issueToDocument(fullName, issue, access)
		out <- interfaces.NewDocResult(&doc)
	}
	if earlyBreak || next == 0 {
		return true
	}
	cp.CurrPage = next
	return false
}
