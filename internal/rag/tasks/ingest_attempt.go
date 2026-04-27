package tasks

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

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
	db *gorm.DB,
	src *ragmodel.RAGSource,
	a *ragmodel.RAGIndexAttempt,
	stats ingestStats,
	runErr error,
	runnable RunnableCheckpointed,
) error {
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
		// A failed attempt does not pause the source; the next scan
		// tick can pick it up.
		return runErr
	}

	srcUpd := map[string]any{
		"updated_at": now,
	}
	// Don't stamp last_successful_index_time on a zero-doc INITIAL run.
	// The next scheduler tick computes its window from this timestamp;
	// a premature stamp creates a 5-min window that finds nothing,
	// and the source stays "successfully indexed nothing" forever.
	// Only commit the timestamp once we've actually pulled documents
	// or we're past the initial-indexing phase.
	if stats.docsBatched > 0 || src.Status != ragmodel.RAGSourceStatusInitialIndexing {
		srcUpd["last_successful_index_time"] = now
	}
	if src.Status == ragmodel.RAGSourceStatusInitialIndexing && stats.docsBatched > 0 {
		srcUpd["status"] = ragmodel.RAGSourceStatusActive
	}
	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGSource{}).
		Where("id = ?", src.ID).
		Updates(srcUpd).Error; err != nil {
		return fmt.Errorf("finalize source %s: %w", src.ID, err)
	}
	return nil
}

// stampDocsEstimated calls the connector's optional pre-flight count
// and stamps it onto the attempt row so the UI can render a determinate
// progress bar. Failures are logged and ignored — an estimate is a
// nice-to-have, never load-bearing for the run itself.
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

// bumpAttemptProgress increments the running doc counter on the attempt
// row. Called once per flushed batch — per-doc would be too chatty.
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
