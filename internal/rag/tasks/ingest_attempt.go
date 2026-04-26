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

// openAttempt inserts a fresh rag_index_attempts row in IN_PROGRESS
// state. The HeartbeatCounter / LastHeartbeatTime fields stay zero/null
// until the heartbeat goroutine ticks for the first time. CheckpointPointer
// is copied from the latest non-terminal attempt for the same source so
// the connector can resume mid-stream.
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
		// Inherit checkpoint from the most recent terminal attempt for
		// this source, if any. Without this, a crashed attempt cannot
		// resume from where it left off.
		if cp := lastCheckpointPointer(ctx, db, src.ID); cp != "" {
			a.CheckpointPointer = &cp
		}
	}
	if err := db.WithContext(ctx).Create(a).Error; err != nil {
		return nil, fmt.Errorf("ingest: open attempt for %s: %w", src.ID, err)
	}
	return a, nil
}

// lastCheckpointPointer returns the CheckpointPointer of the most
// recent terminal attempt for the source, or "" if none exists. Used
// by openAttempt to seed the checkpoint resume.
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

// finalizeAttempt closes out the attempt row and (on success) advances
// the source's bookkeeping columns. Called exactly once per HandleIngest
// invocation, after the heartbeat goroutine has been signalled to stop.
//
// Decision matrix:
//
//	runErr is fatal       → status=FAILED,   error_msg=runErr
//	stats.failures > 0    → status=COMPLETED_WITH_ERRORS
//	otherwise             → status=SUCCESS
//
// On non-failure, advances source.LastSuccessfulIndexTime and flips
// INITIAL_INDEXING → ACTIVE per Onyx parity.
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

	// Persist the connector's final checkpoint, if it produced one and
	// the run did not fail outright. A failed run keeps the previous
	// checkpoint so the next attempt can resume.
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
		// Source's status is unchanged so the next scan tick can pick
		// it up. Onyx parity: a failed attempt does not pause the source.
		return runErr
	}

	srcUpd := map[string]any{
		"last_successful_index_time": now,
		"updated_at":                 now,
	}
	if src.Status == ragmodel.RAGSourceStatusInitialIndexing {
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

// recordAttemptError persists a per-doc / per-entity failure into
// rag_index_attempt_errors. Errors writing the error row are logged
// (we don't fail the whole attempt over an error-log INSERT failure,
// but we make the failure visible).
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
