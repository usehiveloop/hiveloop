package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	ragtasks "github.com/usehivy/hivy/internal/rag/tasks"
)

var errIngestReservationSkipped = errors.New("ingest reservation skipped")

func ScanIngestDue(
	ctx context.Context,
	db *gorm.DB,
	enq enqueue.TaskEnqueuer,
	cfg Config,
) (int, error) {
	ids, err := selectIngestDueSourceIDs(ctx, db, cfg.EnqueueLimit)
	if err != nil {
		return 0, fmt.Errorf("select ingest-due sources: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}

	enqueued := 0
	var firstErr error
	uniqueTTL := cfg.IngestTick + cfg.UniqueSlack
	for _, id := range ids {
		if ctx.Err() != nil {
			return enqueued, ctx.Err()
		}
		attemptID, err := reserveIngestAttempt(ctx, db, id)
		if err != nil {
			if errors.Is(err, errIngestReservationSkipped) {
				continue
			}
			logging.Capture(ctx, fmt.Errorf("rag scheduler reserve ingest source=%s: %w", id, err))
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := enqueueIngest(enq, id, attemptID, uniqueTTL); err != nil {
			failIngestReservation(ctx, db, attemptID, err)
			logging.Capture(ctx, fmt.Errorf("rag scheduler enqueue ingest source=%s: %w", id, err))
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		enqueued++
	}
	return enqueued, firstErr
}

func selectIngestDueSourceIDs(ctx context.Context, db *gorm.DB, limit int) ([]uuid.UUID, error) {
	const q = `
		SELECT id
		FROM rag_sources s
		WHERE s.enabled = true
		  AND s.status IN ('ACTIVE','INITIAL_INDEXING')
		  AND s.refresh_freq_seconds IS NOT NULL
		  AND (
		    s.last_successful_index_time IS NULL
		    OR s.last_successful_index_time +
		       (s.refresh_freq_seconds * INTERVAL '1 second') < NOW()
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM rag_index_attempts a
		    WHERE a.rag_source_id = s.id
		      AND a.status IN ('not_started', 'in_progress')
		  )
		ORDER BY COALESCE(s.last_successful_index_time, '1970-01-01'::timestamptz)
		LIMIT ?
	`
	var ids []uuid.UUID
	if err := db.WithContext(ctx).Raw(q, limit).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func reserveIngestAttempt(ctx context.Context, db *gorm.DB, sourceID uuid.UUID) (uuid.UUID, error) {
	var attemptID uuid.UUID
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		src, err := lockIngestSource(ctx, tx, sourceID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errIngestReservationSkipped
			}
			return err
		}
		if !sourceIngestDue(src, time.Now()) {
			return errIngestReservationSkipped
		}
		active, err := sourceHasActiveIngestAttempt(ctx, tx, sourceID)
		if err != nil {
			return err
		}
		if active {
			return errIngestReservationSkipped
		}

		attempt := ragmodel.RAGIndexAttempt{
			OrgID:       src.OrgID,
			RAGSourceID: sourceID,
			Status:      ragmodel.IndexingStatusNotStarted,
		}
		if err := tx.Create(&attempt).Error; err != nil {
			return fmt.Errorf("create ingest reservation: %w", err)
		}
		attemptID = attempt.ID
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return attemptID, nil
}

type lockedIngestSource struct {
	ID                      uuid.UUID
	OrgID                   uuid.UUID `gorm:"column:org_id"`
	Enabled                 bool
	Status                  ragmodel.RAGSourceStatus
	RefreshFreqSeconds      *int
	LastSuccessfulIndexTime *time.Time
}

func lockIngestSource(ctx context.Context, tx *gorm.DB, sourceID uuid.UUID) (lockedIngestSource, error) {
	var src lockedIngestSource
	res := tx.WithContext(ctx).Raw(`
		SELECT id, org_id, enabled, status, refresh_freq_seconds, last_successful_index_time
		FROM rag_sources
		WHERE id = ?
		FOR UPDATE
	`, sourceID).Scan(&src)
	if res.Error != nil {
		return lockedIngestSource{}, res.Error
	}
	if res.RowsAffected == 0 {
		return lockedIngestSource{}, gorm.ErrRecordNotFound
	}
	return src, nil
}

func sourceIngestDue(src lockedIngestSource, now time.Time) bool {
	if !src.Enabled {
		return false
	}
	switch src.Status {
	case ragmodel.RAGSourceStatusActive, ragmodel.RAGSourceStatusInitialIndexing:
	default:
		return false
	}
	if src.RefreshFreqSeconds == nil {
		return false
	}
	if src.LastSuccessfulIndexTime == nil {
		return true
	}
	nextRun := src.LastSuccessfulIndexTime.Add(time.Duration(*src.RefreshFreqSeconds) * time.Second)
	return nextRun.Before(now)
}

func sourceHasActiveIngestAttempt(ctx context.Context, tx *gorm.DB, sourceID uuid.UUID) (bool, error) {
	var count int64
	err := tx.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("rag_source_id = ? AND status IN ?", sourceID, []ragmodel.IndexingStatus{
			ragmodel.IndexingStatusNotStarted,
			ragmodel.IndexingStatusInProgress,
		}).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("count active ingest attempts: %w", err)
	}
	return count > 0, nil
}

func failIngestReservation(ctx context.Context, db *gorm.DB, attemptID uuid.UUID, cause error) {
	if attemptID == uuid.Nil {
		return
	}
	now := time.Now()
	msg := cause.Error()
	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("id = ? AND status = ?", attemptID, ragmodel.IndexingStatusNotStarted).
		Updates(map[string]any{
			"status":       ragmodel.IndexingStatusFailed,
			"error_msg":    msg,
			"time_updated": now,
		}).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("rag scheduler fail ingest reservation attempt=%s: %w", attemptID, err))
	}
}

func enqueueIngest(enq enqueue.TaskEnqueuer, sourceID, attemptID uuid.UUID, uniqueTTL time.Duration) error {
	task, err := ragtasks.NewIngestTask(
		ragtasks.IngestPayload{RAGSourceID: sourceID, AttemptID: &attemptID},
	)
	if err != nil {
		return err
	}
	if _, err := enq.Enqueue(task, asynqUnique(uniqueTTL)); err != nil {
		if isDuplicate(err) {
			return nil
		}
		return err
	}
	return nil
}
