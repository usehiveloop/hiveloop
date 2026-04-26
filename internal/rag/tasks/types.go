package tasks

const (
	TypeRagScanIngestDue   = "rag:scan_ingest_due"
	TypeRagScanPermSyncDue = "rag:scan_perm_sync_due"
	TypeRagScanPruneDue    = "rag:scan_prune_due"
	TypeRagWatchdog        = "rag:watchdog_stuck_attempts"

	TypeRagIngest   = "rag:ingest"
	TypeRagPermSync = "rag:perm_sync"
	TypeRagPrune    = "rag:prune"
)

const (
	QueueRagWork = "rag_work"
)
