package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

// ScanPruneDue selects rag_sources whose prune_freq_seconds has elapsed
// since the last prune and whose connector kind supports SlimConnector,
// then enqueues a TypeRagPrune task for each. Mirrors Onyx's
// check_for_pruning at
// backend/onyx/background/celery/tasks/pruning/tasks.py:206-314, gated
// by _is_pruning_due at tasks.py:164-197.
func ScanPruneDue(
	ctx context.Context,
	db *gorm.DB,
	enq enqueue.TaskEnqueuer,
	cfg Config,
	supports CapabilityCheck,
) (int, error) {
	if supports == nil {
		supports = HasSlimCapability
	}
	rows, err := selectPruneDueSources(ctx, db, cfg.EnqueueLimit)
	if err != nil {
		return 0, fmt.Errorf("select prune-due sources: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}

	enqueued := 0
	var firstErr error
	uniqueTTL := cfg.PruneTick + cfg.UniqueSlack
	for _, r := range rows {
		if ctx.Err() != nil {
			return enqueued, ctx.Err()
		}
		if !supports(r.Kind) {
			continue
		}
		task, err := ragtasks.NewPruneTask(
			ragtasks.PrunePayload{RAGSourceID: r.ID},
		)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if _, err := enq.Enqueue(task, asynqUnique(uniqueTTL)); err != nil {
			if isDuplicate(err) {
				continue
			}
			slog.Error("rag scheduler: enqueue prune failed",
				"source_id", r.ID, "err", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		enqueued++
	}
	return enqueued, firstErr
}

// pruneCandidate matches the SELECT shape below.
type pruneCandidate struct {
	ID   uuid.UUID
	Kind string
}

// selectPruneDueSources returns sources whose last_pruned + prune_freq_seconds
// has passed (or that have never been pruned). The query intentionally
// does NOT exclude in-flight ingest attempts — pruning is independent
// of the document fetcher and runs against the persisted snapshot.
func selectPruneDueSources(ctx context.Context, db *gorm.DB, limit int) ([]pruneCandidate, error) {
	const q = `
		SELECT id, kind
		FROM rag_sources s
		WHERE s.enabled = true
		  AND s.status IN ('ACTIVE','INITIAL_INDEXING')
		  AND s.prune_freq_seconds IS NOT NULL
		  AND (
		    s.last_pruned IS NULL
		    OR s.last_pruned +
		       (s.prune_freq_seconds * INTERVAL '1 second') < NOW()
		  )
		ORDER BY COALESCE(s.last_pruned, '1970-01-01'::timestamptz)
		LIMIT ?
	`
	var rows []pruneCandidate
	if err := db.WithContext(ctx).Raw(q, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
