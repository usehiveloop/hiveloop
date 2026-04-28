package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehiveloop/hiveloop/internal/enqueue"
	"github.com/usehiveloop/hiveloop/internal/rag/connectors/interfaces"
	ragtasks "github.com/usehiveloop/hiveloop/internal/rag/tasks"
)

type CapabilityCheck func(kind string) bool

func HasPermSyncCapability(kind string) bool {
	factory, err := interfaces.Lookup(kind)
	if err != nil {
		return false
	}
	c, err := factory(nilSource{kind: kind}, interfaces.BuildDeps{})
	if err != nil || c == nil {
		return false
	}
	_, ok := c.(interfaces.PermSyncConnector)
	return ok
}

func HasSlimCapability(kind string) bool {
	factory, err := interfaces.Lookup(kind)
	if err != nil {
		return false
	}
	c, err := factory(nilSource{kind: kind}, interfaces.BuildDeps{})
	if err != nil || c == nil {
		return false
	}
	_, ok := c.(interfaces.SlimConnector)
	return ok
}

type nilSource struct{ kind string }

func (s nilSource) SourceID() string        { return "" }
func (s nilSource) OrgID() string           { return "" }
func (s nilSource) SourceKind() string      { return s.kind }
func (s nilSource) Config() json.RawMessage { return json.RawMessage(`{}`) }

func ScanPermSyncDue(
	ctx context.Context,
	db *gorm.DB,
	enq enqueue.TaskEnqueuer,
	cfg Config,
	supports CapabilityCheck,
) (int, error) {
	if supports == nil {
		supports = HasPermSyncCapability
	}
	rows, err := selectPermSyncDueSources(ctx, db, cfg.EnqueueLimit)
	if err != nil {
		return 0, fmt.Errorf("select perm-sync-due sources: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}

	enqueued := 0
	var firstErr error
	uniqueTTL := cfg.PermSyncTick + cfg.UniqueSlack
	for _, r := range rows {
		if ctx.Err() != nil {
			return enqueued, ctx.Err()
		}
		if !supports(r.Kind) {
			continue
		}
		task, err := ragtasks.NewPermSyncTask(
			ragtasks.PermSyncPayload{RAGSourceID: r.ID},
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
			slog.Error("rag scheduler: enqueue perm_sync failed",
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

type permSyncCandidate struct {
	ID   uuid.UUID
	Kind string
}

func selectPermSyncDueSources(ctx context.Context, db *gorm.DB, limit int) ([]permSyncCandidate, error) {
	const q = `
		SELECT id, kind
		FROM rag_sources s
		WHERE s.enabled = true
		  AND s.status IN ('ACTIVE','INITIAL_INDEXING')
		  AND s.access_type = 'sync'
		  AND s.perm_sync_freq_seconds IS NOT NULL
		  AND (
		    s.last_time_perm_sync IS NULL
		    OR s.last_time_perm_sync +
		       (s.perm_sync_freq_seconds * INTERVAL '1 second') < NOW()
		  )
		ORDER BY COALESCE(s.last_time_perm_sync, '1970-01-01'::timestamptz)
		LIMIT ?
	`
	var rows []permSyncCandidate
	if err := db.WithContext(ctx).Raw(q, limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
