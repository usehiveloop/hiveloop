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

type pruneCandidate struct {
	ID   uuid.UUID
	Kind string
}

// Pruning is independent of in-flight ingest attempts — it runs against
// the persisted snapshot, not the live fetcher.
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
