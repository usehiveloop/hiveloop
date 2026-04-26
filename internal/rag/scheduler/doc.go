// Package scheduler implements the four-loop driver that turns RAGSource
// rows into Asynq jobs: ingest, permission sync, prune, and stuck-attempt
// recovery (watchdog).
//
// Each loop runs as a periodic Asynq task. The handler scans rag_sources
// (or rag_index_attempts, for the watchdog) with a tight predicate and
// enqueues per-source tasks via asynq.Unique so duplicate scans within
// a TTL window do not produce duplicate jobs. The actual work — driving
// connectors, calling ragclient — lives in internal/rag/tasks.
//
// Onyx mapping (high-level):
//
//	scan ingest      → backend/onyx/background/celery/tasks/docprocessing/tasks.py:788-1149 (check_for_indexing)
//	scan perm sync   → backend/ee/onyx/background/celery/tasks/doc_permission_syncing/tasks.py:188-288
//	scan prune       → backend/onyx/background/celery/tasks/pruning/tasks.py:206-314
//	watchdog         → backend/onyx/background/celery/tasks/docprocessing/tasks.py:294-385 (monitor_indexing_attempt_progress)
//
// The package exports four Scan* functions plus a Configs() helper that
// returns the asynq.PeriodicTaskConfig list to register with the scheduler.
package scheduler
