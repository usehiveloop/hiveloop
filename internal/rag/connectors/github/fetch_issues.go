package github

import (
	"context"
	"errors"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

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
		// GitHub returns PRs on /repos/.../issues; they're handled by
		// the /pulls endpoint instead.
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
