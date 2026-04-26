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

// ScanIngestDue selects every RAGSource that is enabled, in an active
// status, has a non-null refresh frequency, is not currently being
// ingested, and whose last successful ingest is older than its refresh
// frequency. For each match it enqueues a TypeRagIngest task with
// asynq.Unique keyed on the source ID so duplicate scans within the
// configured slack window collapse to a single job.
//
// Port of Onyx check_for_indexing at
// backend/onyx/background/celery/tasks/docprocessing/tasks.py:788-1149.
// The predicate mirrors should_index() at
// backend/onyx/background/celery/tasks/docprocessing/utils.py:171-300.
//
// Returns the number of tasks enqueued. A non-nil error indicates a DB
// or enqueue failure on at least one source; partial progress is still
// reported via the count.
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

// selectIngestDueSourceIDs returns the IDs of rag_sources rows that are
// eligible for an ingest run right now. The query is bounded by limit
// to keep a single tick's work proportional to the worker pool, even at
// thousands of sources.
//
// Predicate breakdown:
//   - enabled = true                           — admin toggle
//   - status IN ('ACTIVE','INITIAL_INDEXING')  — source is live
//   - refresh_freq_seconds IS NOT NULL         — null = on-demand only
//   - last_successful_index_time IS NULL
//     OR last_successful_index_time +
//        refresh_freq_seconds * '1s' < NOW()   — overdue
//   - NOT EXISTS in-flight attempt             — one concurrent run max
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

// enqueueIngest builds and submits a single TypeRagIngest task with a
// Unique TTL keyed on the typename + payload so duplicate scans inside
// the window collapse.
func enqueueIngest(enq enqueue.TaskEnqueuer, sourceID uuid.UUID, uniqueTTL time.Duration) error {
	task, err := ragtasks.NewIngestTask(
		ragtasks.IngestPayload{RAGSourceID: sourceID},
	)
	if err != nil {
		return err
	}
	if _, err := enq.Enqueue(task,
		// Unique keys on (typename, payload) so the same source ID
		// inside the same window is a no-op.
		asynqUnique(uniqueTTL),
	); err != nil {
		// asynq returns ErrDuplicateTask when Unique blocks the
		// enqueue; that's the success case for our dedupe contract.
		if isDuplicate(err) {
			return nil
		}
		return err
	}
	return nil
}
