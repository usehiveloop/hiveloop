package tasks

// Task type constants for the RAG-driven Asynq tasks. Names use the
// `rag:` prefix so the asynqmon dashboard groups them under the RAG
// subsystem.
const (
	// Periodic scan tasks. Enqueued by the asynq scheduler; handled by
	// the scheduler package.
	TypeRagScanIngestDue   = "rag:scan_ingest_due"
	TypeRagScanPermSyncDue = "rag:scan_perm_sync_due"
	TypeRagScanPruneDue    = "rag:scan_prune_due"
	TypeRagWatchdog        = "rag:watchdog_stuck_attempts"

	// Per-source work tasks. Enqueued by the scan tasks above; handled
	// by the per-source handlers in this package.
	TypeRagIngest   = "rag:ingest"
	TypeRagPermSync = "rag:perm_sync"
	TypeRagPrune    = "rag:prune"
)

// Queue names for the RAG worker pool. All four use the same `rag:work`
// queue today; queue-priority tuning is deferred to ops.
const (
	QueueRagWork = "rag_work"
)
