package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

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
		if err := enqueueIngest(enq, id, uniqueTTL); err != nil {
			slog.Error("rag scheduler: enqueue ingest failed",
				"source_id", id, "err", err)
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
		      AND a.status = 'in_progress'
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

func enqueueIngest(enq enqueue.TaskEnqueuer, sourceID uuid.UUID, uniqueTTL time.Duration) error {
	task, err := ragtasks.NewIngestTask(
		ragtasks.IngestPayload{RAGSourceID: sourceID},
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
