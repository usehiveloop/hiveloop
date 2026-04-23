package model

import "gorm.io/gorm"

// createIndexes installs the indexes that cannot be expressed via gorm
// struct tags: partial indexes, GIN indexes, composite indexes with
// DESC columns, and unique partial indexes. Every statement uses
// IF NOT EXISTS so re-running is a no-op.
func createIndexes(db *gorm.DB) error {
	stmts := []string{
		// ----- rag_documents -----

		// Partial index driving the sync loop: the scheduler scans for
		// documents whose server-side state changed after their last
		// successful write to the vector store. Port of Onyx's
		// ix_document_needs_sync at backend/onyx/db/models.py:1058-1062.
		`CREATE INDEX IF NOT EXISTS idx_rag_document_needs_sync
		 ON rag_documents (id)
		 WHERE last_modified > last_synced OR last_synced IS NULL`,

		// GIN index for ACL filtering by email. Without it, permission
		// sync and admin-audit queries over external_user_emails do a
		// full sequential scan.
		`CREATE INDEX IF NOT EXISTS idx_rag_document_ext_emails
		 ON rag_documents USING GIN (external_user_emails)`,

		// GIN index for ACL filtering by group id. Same rationale as
		// the email index.
		`CREATE INDEX IF NOT EXISTS idx_rag_document_ext_group_ids
		 ON rag_documents USING GIN (external_user_group_ids)`,

		// ----- rag_hierarchy_nodes -----

		// Prevents the same upstream page being ingested under two
		// different HierarchyNode rows. Port of Onyx's
		// uq_hierarchy_node_raw_id_source at
		// backend/onyx/db/models.py:930-932.
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_rag_hierarchy_node_raw_id_source
		 ON rag_hierarchy_nodes (raw_node_id, source)`,

		// ----- rag_index_attempts -----

		// "Latest attempt per source" lookup used by the scheduler.
		// Port of Onyx's ix_index_attempt_latest_for_connector_credential_pair
		// at backend/onyx/db/models.py:2297-2301.
		`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_latest_for_source
		 ON rag_index_attempts (rag_source_id, time_created)`,

		// "Latest by source + embedding model" lookup; DESC on
		// time_updated turns the query into a one-row index scan. Port
		// of Onyx's ix_index_attempt_ccpair_search_settings_time_updated
		// at backend/onyx/db/models.py:2302-2308.
		`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_source_model_updated
		 ON rag_index_attempts (rag_source_id, embedding_model_id, time_updated DESC)`,

		// Drives the scheduler's "latest-by-status" poll that runs every
		// 15s. Port of Onyx's ix_index_attempt_cc_pair_settings_poll at
		// backend/onyx/db/models.py:2309-2315.
		`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_source_model_poll
		 ON rag_index_attempts (rag_source_id, embedding_model_id, status, time_updated DESC)`,

		// Active-attempt coordination: before spawning a new attempt,
		// the scheduler checks whether a live one already exists. Port
		// of Onyx's ix_index_attempt_active_coordination at
		// backend/onyx/db/models.py:2317-2322.
		`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_active_coord
		 ON rag_index_attempts (rag_source_id, embedding_model_id, status)`,

		// Watchdog partial index. The stall detector scans for
		// in-progress attempts whose last heartbeat is older than the
		// watchdog threshold. The partial predicate keeps the index
		// tiny even when historical attempt volume grows.
		`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_heartbeat
		 ON rag_index_attempts (status, last_progress_time)
		 WHERE status = 'in_progress'`,

		// ----- rag_sync_records -----

		// "Latest syncs for this entity" pagination in the admin UI.
		// Port of Onyx's ix_sync_record_entity_id_sync_type_sync_start_time
		// at backend/onyx/db/models.py:2465-2470.
		`CREATE INDEX IF NOT EXISTS idx_rag_sync_record_entity_type_start
		 ON rag_sync_records (entity_id, sync_type, sync_start_time)`,

		// "Is there an in-progress sync of this type for this entity?"
		// — prevents duplicate concurrent syncs. Port of Onyx's
		// ix_sync_record_entity_id_sync_type_sync_status at
		// backend/onyx/db/models.py:2471-2476.
		`CREATE INDEX IF NOT EXISTS idx_rag_sync_record_entity_type_status
		 ON rag_sync_records (entity_id, sync_type, sync_status)`,

		// ----- rag_sync_states -----

		// Org-wide status scan for admin dashboards and scheduler
		// gating (e.g. "show me all paused sources in org X").
		`CREATE INDEX IF NOT EXISTS idx_rag_sync_state_org_status
		 ON rag_sync_states (org_id, status)`,

		// ----- rag_external_user_groups (junction stale-sweep) -----

		// Used by the permission-sync stale sweep when finalising a
		// sync for a source. Port of Onyx's ix_user_external_group_cc_pair_stale
		// at backend/onyx/db/models.py:4340-4344.
		`CREATE INDEX IF NOT EXISTS idx_rag_user_external_group_source_stale
		 ON rag_user_external_user_groups (rag_source_id, stale)`,

		// Org-wide stale sweep fallback. Port of Onyx's
		// ix_user_external_group_stale at models.py:4345-4348.
		`CREATE INDEX IF NOT EXISTS idx_rag_user_external_group_stale
		 ON rag_user_external_user_groups (stale)`,

		// Stale sweep for anyone-with-the-link style public shares.
		// Port of Onyx's ix_public_external_group_cc_pair_stale at
		// backend/onyx/db/models.py:4371-4375.
		`CREATE INDEX IF NOT EXISTS idx_rag_public_external_group_source_stale
		 ON rag_public_external_user_groups (rag_source_id, stale)`,

		// Org-wide stale sweep fallback for the public variant. Port
		// of Onyx's ix_public_external_group_stale at models.py:4376-4379.
		`CREATE INDEX IF NOT EXISTS idx_rag_public_external_group_stale
		 ON rag_public_external_user_groups (stale)`,

		// ----- rag_sources -----

		// Partial unique index guaranteeing at most one RAGSource per
		// InConnection, scoped to INTEGRATION-kind rows (WEBSITE /
		// FILE_UPLOAD rows carry in_connection_id IS NULL and so
		// shouldn't collide).
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_rag_sources_in_connection
		 ON rag_sources (in_connection_id)
		 WHERE kind = 'INTEGRATION'`,

		// Admin-dashboard scan: "list all my sources grouped by status".
		`CREATE INDEX IF NOT EXISTS idx_rag_sources_org_status
		 ON rag_sources (org_id, status)`,

		// Partial index the scheduler scans to pick work. Excludes
		// paused / errored / deleting sources so the hot-path query
		// stays small even when an org has thousands of sources.
		`CREATE INDEX IF NOT EXISTS idx_rag_sources_needs_ingest
		 ON rag_sources (org_id)
		 WHERE enabled = true AND status IN ('ACTIVE','INITIAL_INDEXING')`,

		// Prune-loop scan: surfaces sources that have ever been pruned
		// so the next-pruning-due query doesn't walk ones that never
		// have been.
		`CREATE INDEX IF NOT EXISTS idx_rag_sources_last_pruned
		 ON rag_sources (last_pruned)
		 WHERE last_pruned IS NOT NULL`,
	}

	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}
