// PR-fetch loop: descend pages of /repos/{owner}/{repo}/pulls in
// reverse-chronological updated_at order, emit Documents, early-break
// when the page ends below the start window.
//
// Onyx analog: connector.py:611-690 (`_fetch_pull_requests`). Match the
// state-filter, the sort, the per_page=100, and the early-break — but
// pagination is server-driven (Link header) instead of Onyx's manual
// page counter.
package github

import (
	"context"
	"errors"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// fetchPRsPage drains exactly one page of PRs and emits Documents +
// per-doc failures into out. Returns true when the repo is done at this
// stage (last page or early-break on time window); false when the
// caller should loop and fetch the next page.
//
// Time-window contract:
//   - end == zero or PR.UpdatedAt > end : skip the PR (not yet in window).
//   - PR.UpdatedAt < start              : early-break, return done=true.
//   - else                              : convert + emit.
func fetchPRsPage(
	ctx context.Context,
	client *Client,
	fullName, state string,
	cp *GithubCheckpoint,
	start, end time.Time,
	access *interfaces.ExternalAccess,
	out chan<- interfaces.DocumentOrFailure,
) bool {
	prs, next, err := client.listPullRequestsPage(ctx, fullName, state, cp.CurrPage)
	if err != nil {
		// Rate-limit exhaustion or transport errors are repo-scoped
		// (entire page failed) — emit an entity failure and stop the
		// repo at this stage.
		if errors.Is(err, errRateLimited) {
			out <- interfaces.NewDocFailure(entityFailure(fullName,
				"github: pull requests page exhausted retries", err))
		} else {
			out <- interfaces.NewDocFailure(entityFailure(fullName,
				"github: pull requests page fetch failed", err))
		}
		return true
	}

	earlyBreak := false
	for i := range prs {
		pr := prs[i]
		// Window filter — both ends are honoured, with the start side
		// triggering early-break (page is sorted updated-desc, so
		// anything older than start means the rest of the page + every
		// later page is older still).
		if !end.IsZero() && pr.UpdatedAt.After(end) {
			continue
		}
		if !start.IsZero() && pr.UpdatedAt.Before(start) {
			earlyBreak = true
			break
		}
		// Track the most recent UpdatedAt seen — used by checkpoint
		// resume to short-circuit re-fetches on early pages.
		if cp.LastSeenUpdatedAt == nil || pr.UpdatedAt.After(*cp.LastSeenUpdatedAt) {
			t := pr.UpdatedAt
			cp.LastSeenUpdatedAt = &t
		}
		doc := prToDocument(fullName, pr, access)
		out <- interfaces.NewDocResult(&doc)
	}
	if earlyBreak || next == 0 {
		return true
	}
	cp.CurrPage = next
	return false
}
