package tasks

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

var errIngestAttemptAlreadyClaimed = errors.New("ingest attempt already claimed")

func openAttempt(
	ctx context.Context,
	db *gorm.DB,
	src *ragmodel.RAGSource,
	fromBeginning bool,
	attemptID *uuid.UUID,
) (*ragmodel.RAGIndexAttempt, error) {
	if attemptID != nil {
		return claimReservedAttempt(ctx, db, src, *attemptID)
	}
	return createAdHocAttempt(ctx, db, src, fromBeginning)
}

func createAdHocAttempt(
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
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSourceForAttempt(ctx, tx, src.ID); err != nil {
			return err
		}
		active, err := activeAttemptExists(ctx, tx, src.ID, nil)
		if err != nil {
			return err
		}
		if active {
			return errIngestAttemptAlreadyClaimed
		}
		if !fromBeginning {
			if cp := lastCheckpointPointer(ctx, tx, src.ID); cp != "" {
				a.CheckpointPointer = &cp
			}
		}
		if err := tx.WithContext(ctx).Create(a).Error; err != nil {
			return fmt.Errorf("create attempt: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errIngestAttemptAlreadyClaimed) {
			return nil, err
		}
		return nil, fmt.Errorf("ingest: open attempt for %s: %w", src.ID, err)
	}
	return a, nil
}

func claimReservedAttempt(
	ctx context.Context,
	db *gorm.DB,
	src *ragmodel.RAGSource,
	attemptID uuid.UUID,
) (*ragmodel.RAGIndexAttempt, error) {
	now := time.Now()
	var attempt ragmodel.RAGIndexAttempt
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSourceForAttempt(ctx, tx, src.ID); err != nil {
			return err
		}
		active, err := activeAttemptExists(ctx, tx, src.ID, &attemptID)
		if err != nil {
			return err
		}
		if active {
			return errIngestAttemptAlreadyClaimed
		}

		res := tx.Model(&ragmodel.RAGIndexAttempt{}).
			Where("id = ? AND rag_source_id = ? AND status = ?", attemptID, src.ID, ragmodel.IndexingStatusNotStarted).
			Updates(map[string]any{
				"status":             ragmodel.IndexingStatusInProgress,
				"last_progress_time": now,
				"time_started":       now,
				"time_updated":       now,
			})
		if res.Error != nil {
			return fmt.Errorf("claim reserved attempt: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			if err := tx.First(&attempt, "id = ? AND rag_source_id = ?", attemptID, src.ID).Error; err != nil {
				return fmt.Errorf("load reserved attempt: %w", err)
			}
			return errIngestAttemptAlreadyClaimed
		}
		if err := tx.First(&attempt, "id = ?", attemptID).Error; err != nil {
			return fmt.Errorf("load claimed attempt: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errIngestAttemptAlreadyClaimed) {
			return nil, err
		}
		return nil, fmt.Errorf("ingest: claim reserved attempt %s for %s: %w", attemptID, src.ID, err)
	}
	return &attempt, nil
}

func lockSourceForAttempt(ctx context.Context, tx *gorm.DB, sourceID uuid.UUID) error {
	var id string
	res := tx.WithContext(ctx).Raw("SELECT id FROM rag_sources WHERE id = ? FOR UPDATE", sourceID).Scan(&id)
	if res.Error != nil {
		return fmt.Errorf("lock source %s: %w", sourceID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("lock source %s: %w", sourceID, gorm.ErrRecordNotFound)
	}
	return nil
}

func activeAttemptExists(ctx context.Context, tx *gorm.DB, sourceID uuid.UUID, exceptID *uuid.UUID) (bool, error) {
	q := tx.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("rag_source_id = ? AND status IN ?", sourceID, []ragmodel.IndexingStatus{
			ragmodel.IndexingStatusNotStarted,
			ragmodel.IndexingStatusInProgress,
		})
	if exceptID != nil {
		q = q.Where("id <> ?", *exceptID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, fmt.Errorf("count active attempts: %w", err)
	}
	return count > 0, nil
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
// (rag_source_attempt_credit, attempt.ID) — retries can't double-charge.
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
		logging.Capture(ctx, fmt.Errorf("rag ingest estimate total source=%s attempt=%s: %w", src.ID, attempt.ID, err))
		return
	}
	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("id = ?", attempt.ID).
		Update("docs_estimated", count).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("rag ingest persist docs_estimated attempt=%s: %w", attempt.ID, err))
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
		logging.Capture(ctx, fmt.Errorf("rag ingest bump progress attempt=%s: %w", attemptID, err))
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
		logging.Capture(ctx, fmt.Errorf("rag ingest record attempt error attempt=%s: %w", attemptID, err))
	}
}
