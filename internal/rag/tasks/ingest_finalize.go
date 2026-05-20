package tasks

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/logging"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

func finalizeAttempt(
	ctx context.Context,
	deps *Deps,
	src *ragmodel.RAGSource,
	a *ragmodel.RAGIndexAttempt,
	stats ingestStats,
	runErr error,
	runnable RunnableCheckpointed,
) error {
	db := deps.DB
	now := time.Now()
	updates := map[string]any{
		"time_updated":       now,
		"new_docs_indexed":   stats.docsBatched,
		"total_docs_indexed": stats.docsBatched,
		"poll_range_start":   stats.pollStart,
		"poll_range_end":     stats.pollEnd,
	}

	terminal := ragmodel.IndexingStatusSuccess
	switch {
	case runErr != nil:
		terminal = ragmodel.IndexingStatusFailed
		msg := fatal(runErr).Error()
		updates["error_msg"] = msg
	case stats.failures > 0:
		terminal = ragmodel.IndexingStatusCompletedWithErrors
	}
	updates["status"] = terminal

	if runErr == nil && runnable != nil {
		if cp, err := runnable.FinalCheckpoint(); err == nil && len(cp) > 0 {
			s := string(cp)
			updates["checkpoint_pointer"] = s
		}
	}

	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("id = ?", a.ID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("finalize attempt %s: %w", a.ID, err)
	}

	if terminal == ragmodel.IndexingStatusFailed {

		if src.Status == ragmodel.RAGSourceStatusInitialIndexing {
			if err := db.WithContext(ctx).
				Model(&ragmodel.RAGSource{}).
				Where("id = ?", src.ID).
				Update("status", ragmodel.RAGSourceStatusError).Error; err != nil {
				logging.Capture(ctx, fmt.Errorf("rag finalize flip INITIAL_INDEXING->ERROR source=%s: %w", src.ID, err))
			}
		}
		return runErr
	}

	srcUpd := map[string]any{
		"updated_at": now,
	}

	if stats.docsBatched > 0 || src.Status != ragmodel.RAGSourceStatusInitialIndexing {
		srcUpd["last_successful_index_time"] = now
	}
	if n, err := deps.Qdrant.CountBySourceID(ctx, deps.Collection, src.ID.String()); err == nil {
		if n > uint64(math.MaxInt32) {
			n = uint64(math.MaxInt32)
		}
		srcUpd["total_docs_indexed"] = int(n) //nolint:gosec // bounded above
	} else {
		logging.Capture(ctx, fmt.Errorf("rag finalize count by source=%s: %w", src.ID, err))
	}
	if (src.Status == ragmodel.RAGSourceStatusInitialIndexing ||
		src.Status == ragmodel.RAGSourceStatusError) && stats.docsBatched > 0 {
		srcUpd["status"] = ragmodel.RAGSourceStatusActive
	}
	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGSource{}).
		Where("id = ?", src.ID).
		Updates(srcUpd).Error; err != nil {
		return fmt.Errorf("finalize source %s: %w", src.ID, err)
	}
	debitWebsiteCredits(ctx, deps, src, a, stats)
	return nil
}

// debitWebsiteCredits charges the org for a website crawl. Idempotent on
// (rag_source_attempt_credit, attempt.ID); retries can't double-charge.
func debitWebsiteCredits(ctx context.Context, deps *Deps, src *ragmodel.RAGSource, a *ragmodel.RAGIndexAttempt, stats ingestStats) {
	if deps.Credits == nil || src.KindValue != ragmodel.RAGSourceKindWebsite || stats.docsBatched <= 0 {
		return
	}
	amount := int64(stats.docsBatched) * billing.WebsitePagePriceCredits
	err := deps.Credits.Spend(
		src.OrgIDValue, amount,
		"rag_website_crawl", "rag_source_attempt_credit", a.ID.String(),
	)
	if err != nil && !errors.Is(err, billing.ErrAlreadyRecorded) {
		logging.FromContext(ctx).WarnContext(ctx, "rag finalize: credit spend failed",
			"source_id", src.ID, "attempt_id", a.ID, "amount", amount, "error", err)
	}
}
