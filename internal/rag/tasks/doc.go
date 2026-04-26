// Package tasks contains the asynq handlers for per-source RAG work:
// ingest, permission sync, and prune. Each handler is dispatched by the
// scheduler in internal/rag/scheduler and drives one Connector to
// completion: loading checkpoint state, streaming Documents, pushing
// batches through ragclient.IngestBatch, and writing metadata into
// rag_documents / rag_index_attempts.
//
// Onyx mapping (high-level):
//
//	ingest   → backend/onyx/background/celery/tasks/docfetching/tasks.py:103-258
//	perm_sync → backend/ee/onyx/background/celery/tasks/doc_permission_syncing/tasks.py
//	prune    → backend/onyx/background/celery/tasks/pruning/tasks.py
//	heartbeat → backend/onyx/background/celery/tasks/docfetching/tasks.py:312-682
//
// The package surface is small: the three Handle functions, a Deps
// struct that bundles their shared dependencies, and the payload
// constructors that the scheduler uses to enqueue.
package tasks
