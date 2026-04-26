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

// CapabilityCheck reports whether the connector kind supports a given
// capability (perm sync, slim listing, …). Injected so tests can
// register stub kinds without touching the global connector registry.
//
// The default production implementation looks up the factory in the
// connector registry, instantiates a throwaway connector with a
// minimal Source view, and type-asserts for the desired capability.
type CapabilityCheck func(kind string) bool

// HasPermSyncCapability is the default CapabilityCheck for perm-sync
// scanning. It looks up the registered factory for kind and checks
// whether the produced Connector additionally satisfies
// interfaces.PermSyncConnector. Sources whose connector kind is not
// registered (or doesn't support perm sync) are skipped.
//
// We pass nil for the Source argument because capability detection is
// an interface-level concern and connectors that need to inspect the
// source for capability are misuse — a perm-sync-capable kind is
// perm-sync-capable for every source of that kind.
func HasPermSyncCapability(kind string) bool {
	factory, err := interfaces.Lookup(kind)
	if err != nil {
		return false
	}
	c, err := factory(nilSource{kind: kind}, nil)
	if err != nil || c == nil {
		return false
	}
	_, ok := c.(interfaces.PermSyncConnector)
	return ok
}

// HasSlimCapability is the default CapabilityCheck used by the prune
// loop. Same shape as HasPermSyncCapability but checks for SlimConnector.
func HasSlimCapability(kind string) bool {
	factory, err := interfaces.Lookup(kind)
	if err != nil {
		return false
	}
	c, err := factory(nilSource{kind: kind}, nil)
	if err != nil || c == nil {
		return false
	}
	_, ok := c.(interfaces.SlimConnector)
	return ok
}

// nilSource is a minimal interfaces.Source for capability probes.
// Production capability detection uses only the kind; UUIDs and config
// are immaterial to the type assertion.
type nilSource struct{ kind string }

func (s nilSource) SourceID() string         { return "" }
func (s nilSource) OrgID() string            { return "" }
func (s nilSource) SourceKind() string       { return s.kind }
func (s nilSource) Config() json.RawMessage  { return json.RawMessage(`{}`) }

// ScanPermSyncDue scans rag_sources whose perm_sync_freq_seconds has
// elapsed since the last sync and whose connector kind supports
// PermSyncConnector. Mirrors check_for_doc_permissions_sync at
// backend/ee/onyx/background/celery/tasks/doc_permission_syncing/tasks.py:188-288.
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

// permSyncCandidate is the minimal projection the scan needs.
type permSyncCandidate struct {
	ID   uuid.UUID
	Kind string
}

// selectPermSyncDueSources runs the gating predicate. It does not
// filter by connector capability — that's done in Go because the set
// of kinds with perm-sync support is small and changes only when a new
// connector lands.
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
