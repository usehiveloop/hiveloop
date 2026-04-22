package model

import "gorm.io/gorm"

// raw-SQL index DDL for Tranche 1B. Split into a package-level slice so
// AutoMigrate1B's control flow is linear (loop + return) instead of
// branchful per statement. The test suite can then cover AutoMigrate1B
// fully without having to inject a broken connection to exercise each
// error-return.
//
// Each entry is a single CREATE INDEX IF NOT EXISTS; idempotent under
// repeated AutoMigrate1B calls. Onyx source references live in the
// comments next to each entry.
var tranche1BIndexDDL = []string{
	// -------------------------------------------------------------------
	// rag_index_attempts — direct ports of Onyx models.py:2296-2322.
	// -------------------------------------------------------------------

	// Port of ix_index_attempt_latest_for_connector_credential_pair
	// (Onyx models.py:2297-2301). Lets the scheduler find the most
	// recent attempt per connection in O(log n).
	`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_latest_for_conn
	 ON rag_index_attempts (in_connection_id, time_created)`,

	// Port of ix_index_attempt_ccpair_search_settings_time_updated
	// (Onyx models.py:2302-2308). DESC on time_updated so "latest by
	// model for this connection" is a one-row index scan.
	`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_conn_model_updated
	 ON rag_index_attempts (in_connection_id, embedding_model_id, time_updated DESC)`,

	// Port of ix_index_attempt_cc_pair_settings_poll
	// (Onyx models.py:2309-2315). Supports the "latest-by-status" poll
	// the scheduler runs every 15s (see Celery beat schedule).
	`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_conn_model_poll
	 ON rag_index_attempts (in_connection_id, embedding_model_id, status, time_updated DESC)`,

	// Port of ix_index_attempt_active_coordination
	// (Onyx models.py:2317-2322). Used by the coordination query that
	// decides whether an attempt is still "live" before spawning a
	// sibling attempt.
	`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_active_coord
	 ON rag_index_attempts (in_connection_id, embedding_model_id, status)`,

	// Hiveloop addition — watchdog partial index. Phase 2's stall
	// detector scans WHERE status='in_progress' AND
	// last_progress_time < NOW() - INTERVAL '30 minutes'. A partial
	// index keyed only on in_progress rows stays tiny regardless of
	// historical attempt volume.
	`CREATE INDEX IF NOT EXISTS idx_rag_index_attempt_heartbeat
	 ON rag_index_attempts (status, last_progress_time)
	 WHERE status = 'in_progress'`,

	// -------------------------------------------------------------------
	// rag_sync_records — direct ports of Onyx models.py:2465-2477.
	// -------------------------------------------------------------------

	// Port of ix_sync_record_entity_id_sync_type_sync_start_time
	// (Onyx models.py:2465-2470). Lets the admin UI page "latest syncs
	// for this entity" efficiently.
	`CREATE INDEX IF NOT EXISTS idx_rag_sync_record_entity_type_start
	 ON rag_sync_records (entity_id, sync_type, sync_start_time)`,

	// Port of ix_sync_record_entity_id_sync_type_sync_status
	// (Onyx models.py:2471-2476). Supports the "is there an
	// in-progress sync of this type for this entity?" scheduler query
	// that prevents duplicate concurrent syncs.
	`CREATE INDEX IF NOT EXISTS idx_rag_sync_record_entity_type_status
	 ON rag_sync_records (entity_id, sync_type, sync_status)`,
}

// AutoMigrate1B runs Tranche 1B's schema migrations: three gorm-managed
// tables (rag_index_attempts, rag_index_attempt_errors,
// rag_sync_records) plus the raw-SQL indexes that gorm tags can't
// express cleanly (composite, DESC, and partial indexes).
//
// Idempotent — every DDL uses IF NOT EXISTS. Tranche 1F's finaliser
// calls this from rag.AutoMigrate alongside the other tranches.
// AutoMigrate1B does NOT register itself anywhere; the finaliser owns
// the call sequence.
func AutoMigrate1B(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&RAGIndexAttempt{},
		&RAGIndexAttemptError{},
		&RAGSyncRecord{},
	); err != nil {
		return err
	}
	for _, ddl := range tranche1BIndexDDL {
		if err := db.Exec(ddl).Error; err != nil {
			return err
		}
	}
	return nil
}
