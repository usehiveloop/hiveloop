package github

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
)

// fetchPRsPage returns true when the repo is done at this stage (last
// page or early-break on time window); false when the caller should
// fetch the next page.
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
	slog.Info("github fetch prs",
		"repo", fullName, "state", state, "page", cp.CurrPage,
		"got", len(prs), "next", next, "err", err)
	if err != nil {
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
	emitted := 0
	skippedAfterEnd := 0
	for i := range prs {
		pr := prs[i]
		if !end.IsZero() && pr.UpdatedAt.After(end) {
			skippedAfterEnd++
			continue
		}
		// Page is sorted updated-desc; anything older than start means
		// every later page is older still.
		if !start.IsZero() && pr.UpdatedAt.Before(start) {
			earlyBreak = true
			break
		}
		if cp.LastSeenUpdatedAt == nil || pr.UpdatedAt.After(*cp.LastSeenUpdatedAt) {
			t := pr.UpdatedAt
			cp.LastSeenUpdatedAt = &t
		}
		doc := prToDocument(fullName, pr, access)
		out <- interfaces.NewDocResult(&doc)
		emitted++
	}
	slog.Info("github prs page emit",
		"repo", fullName, "page", cp.CurrPage,
		"emitted", emitted, "skipped_after_end", skippedAfterEnd,
		"early_break", earlyBreak)
	if earlyBreak || next == 0 {
		return true
	}
	cp.CurrPage = next
	return false
}
