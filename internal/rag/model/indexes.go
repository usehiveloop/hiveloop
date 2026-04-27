package model

import "gorm.io/gorm"

func createIndexes(db *gorm.DB) error {
	stmts := []string{
		`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_latest_for_source
		 ON rag_index_attempts (rag_source_id, time_created)`,

		`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_source_model_updated
		 ON rag_index_attempts (rag_source_id, embedding_model_id, time_updated DESC)`,

		`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_source_model_poll
		 ON rag_index_attempts (rag_source_id, embedding_model_id, status, time_updated DESC)`,

		`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_active_coord
		 ON rag_index_attempts (rag_source_id, embedding_model_id, status)`,

		// Watchdog partial index for stalled in-progress attempts.
		`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_heartbeat
		 ON rag_index_attempts (status, last_progress_time)
		 WHERE status = 'in_progress'`,

		`CREATE INDEX IF NOT EXISTS idx_rag_sync_record_entity_type_start
		 ON rag_sync_records (entity_id, sync_type, sync_start_time)`,

		`CREATE INDEX IF NOT EXISTS idx_rag_sync_record_entity_type_status
		 ON rag_sync_records (entity_id, sync_type, sync_status)`,

		`CREATE INDEX IF NOT EXISTS idx_rag_sync_state_org_status
		 ON rag_sync_states (org_id, status)`,

		`CREATE INDEX IF NOT EXISTS idx_rag_user_external_group_source_stale
		 ON rag_user_external_user_groups (rag_source_id, stale)`,

		`CREATE INDEX IF NOT EXISTS idx_rag_user_external_group_stale
		 ON rag_user_external_user_groups (stale)`,

		`CREATE INDEX IF NOT EXISTS idx_rag_public_external_group_source_stale
		 ON rag_public_external_user_groups (rag_source_id, stale)`,

		`CREATE INDEX IF NOT EXISTS idx_rag_public_external_group_stale
		 ON rag_public_external_user_groups (stale)`,

		// At most one RAGSource per InConnection, scoped to INTEGRATION-kind rows.
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_rag_sources_in_connection
		 ON rag_sources (in_connection_id)
		 WHERE kind = 'INTEGRATION'`,

		`CREATE INDEX IF NOT EXISTS idx_rag_sources_org_status
		 ON rag_sources (org_id, status)`,

		`CREATE INDEX IF NOT EXISTS idx_rag_sources_needs_ingest
		 ON rag_sources (org_id)
		 WHERE enabled = true AND status IN ('ACTIVE','INITIAL_INDEXING')`,

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
