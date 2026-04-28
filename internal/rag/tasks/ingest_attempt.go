package tasks

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/billing"
	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	ragmodel "github.com/usehiveloop/hiveloop/internal/rag/model"
)

func openAttempt(
	ctx context.Context,
	db *gorm.DB,
	src *ragmodel.RAGSource,
	fromBeginning bool,
) (*ragmodel.RAGIndexAttempt, error) {
	now := time.Now()
	a := &ragmodel.RAGIndexAttempt{
		OrgID:            src.OrgIDValue,
		RAGSourceID:      src.ID,
		FromBeginning:    fromBeginning,
		Status:           ragmodel.IndexingStatusInProgress,
		LastProgressTime: &now,
		TimeStarted:      &now,
	}
	if !fromBeginning {
		// Without inheriting the prior attempt's checkpoint a crashed
		// run cannot resume from where it left off.
		if cp := lastCheckpointPointer(ctx, db, src.ID); cp != "" {
			a.CheckpointPointer = &cp
		}
	}
	if err := db.WithContext(ctx).Create(a).Error; err != nil {
		return nil, fmt.Errorf("ingest: open attempt for %s: %w", src.ID, err)
	}
	return a, nil
}

func lastCheckpointPointer(ctx context.Context, db *gorm.DB, sourceID uuid.UUID) string {
	var cp *string
	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("rag_source_id = ? AND checkpoint_pointer IS NOT NULL", sourceID).
		Order("time_created DESC").
		Limit(1).
		Pluck("checkpoint_pointer", &cp).Error; err != nil {
		return ""
	}
	if cp == nil {
		return ""
	}
	return *cp
}

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

	// A failed run keeps the previous checkpoint so the next attempt can resume.
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
		// First-run failure: flip out of INITIAL_INDEXING so the UI
		// stops spinning. Successful retries restore ACTIVE below.
		if src.Status == ragmodel.RAGSourceStatusInitialIndexing {
			if err := db.WithContext(ctx).
				Model(&ragmodel.RAGSource{}).
				Where("id = ?", src.ID).
				Update("status", ragmodel.RAGSourceStatusError).Error; err != nil {
				slog.Warn("rag finalize: flip INITIAL_INDEXING → ERROR failed",
					"source_id", src.ID, "err", err)
			}
		}
		return runErr
	}

	srcUpd := map[string]any{
		"updated_at": now,
	}
	// last_successful_index_time defines the next poll window's lower
	// bound. A 0-doc INITIAL run that stamps it traps the source in a
	// 5-min window that finds nothing on every subsequent tick.
	if stats.docsBatched > 0 || src.Status != ragmodel.RAGSourceStatusInitialIndexing {
		srcUpd["last_successful_index_time"] = now
	}
	if n, err := deps.Qdrant.CountBySourceID(ctx, deps.Collection, src.ID.String()); err == nil {
		if n > uint64(math.MaxInt32) {
			n = uint64(math.MaxInt32)
		}
		srcUpd["total_docs_indexed"] = int(n) //nolint:gosec // bounded above
	} else {
		slog.Warn("rag finalize: count by source failed; leaving total_docs_indexed unchanged",
			"source_id", src.ID, "err", err)
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
	debitWebsiteCredits(deps, src, a, stats)
	return nil
}

// debitWebsiteCredits charges the org for a website crawl. Idempotent on
// (rag_source_attempt_credit, attempt.ID) — retries can't double-charge.
func debitWebsiteCredits(deps *Deps, src *ragmodel.RAGSource, a *ragmodel.RAGIndexAttempt, stats ingestStats) {
	if deps.Credits == nil || src.KindValue != ragmodel.RAGSourceKindWebsite || stats.docsBatched <= 0 {
		return
	}
	amount := int64(stats.docsBatched) * billing.WebsitePagePriceCredits
	err := deps.Credits.Spend(
		src.OrgIDValue, amount,
		"rag_website_crawl", "rag_source_attempt_credit", a.ID.String(),
	)
	if err != nil && !errors.Is(err, billing.ErrAlreadyRecorded) {
		slog.Warn("rag finalize: credit spend failed",
			"source_id", src.ID, "attempt_id", a.ID, "amount", amount, "err", err)
	}
}

func stampDocsEstimated(
	ctx context.Context,
	db *gorm.DB,
	conn interfaces.Connector,
	src *ragmodel.RAGSource,
	attempt *ragmodel.RAGIndexAttempt,
) {
	est, ok := conn.(interfaces.EstimatingConnector)
	if !ok {
		return
	}
	count, err := est.EstimateTotal(ctx, src)
	if err != nil {
		slog.Warn("rag ingest: estimate total failed",
			"source_id", src.ID, "attempt_id", attempt.ID, "err", err)
		return
	}
	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("id = ?", attempt.ID).
		Update("docs_estimated", count).Error; err != nil {
		slog.Warn("rag ingest: persist docs_estimated failed",
			"attempt_id", attempt.ID, "err", err)
		return
	}
	attempt.DocsEstimated = &count
}

func bumpAttemptProgress(
	ctx context.Context,
	db *gorm.DB,
	attemptID uuid.UUID,
	delta int,
) {
	if delta <= 0 {
		return
	}
	now := time.Now()
	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("id = ?", attemptID).
		Updates(map[string]any{
			"new_docs_indexed":   gorm.Expr("COALESCE(new_docs_indexed, 0) + ?", delta),
			"total_docs_indexed": gorm.Expr("COALESCE(total_docs_indexed, 0) + ?", delta),
			"last_progress_time": now,
			"time_updated":       now,
		}).Error; err != nil {
		slog.Warn("rag ingest: bump progress failed",
			"attempt_id", attemptID, "err", err)
	}
}

// We don't fail the whole attempt over an error-log INSERT failure,
// but we surface it via Warn.
func recordAttemptError(
	ctx context.Context,
	db *gorm.DB,
	orgID, sourceID, attemptID uuid.UUID,
	failure *interfaces.ConnectorFailure,
) {
	if failure == nil {
		return
	}
	row := ragmodel.RAGIndexAttemptError{
		OrgID:          orgID,
		IndexAttemptID: attemptID,
		RAGSourceID:    sourceID,
		FailureMessage: failure.FailureMessage,
	}
	if failure.FailedDocument != nil {
		id := failure.FailedDocument.DocID
		row.DocumentID = &id
		if failure.FailedDocument.DocumentLink != "" {
			link := failure.FailedDocument.DocumentLink
			row.DocumentLink = &link
		}
	}
	if failure.FailedEntity != nil {
		eid := failure.FailedEntity.EntityID
		row.EntityID = &eid
		row.FailedTimeRangeStart = failure.FailedEntity.MissedTimeRangeStart
		row.FailedTimeRangeEnd = failure.FailedEntity.MissedTimeRangeEnd
	}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		slog.Warn("rag ingest: record attempt error failed",
			"attempt_id", attemptID, "err", err)
	}
}
