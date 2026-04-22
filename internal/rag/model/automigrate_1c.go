package model

import "gorm.io/gorm"

// AutoMigrate1C runs gorm.AutoMigrate for the three models owned by
// Tranche 1C plus the manual SQL index statements Onyx relies on.
//
// Consumed by Tranche 1F's registration aggregator (see
// `internal/rag/register.go`). Not wired from register.go in this
// tranche — 1F is the single merge point per plan §Launch order.
//
// IMPORTANT: `RAGSearchSettings.EmbeddingModelID` references
// `rag_embedding_models.id` which is owned by Tranche 1G. The
// cross-table FK constraint is NOT added here because 1G's table may
// not exist yet at this call site; 1F is responsible for ordering
// the AutoMigrate calls (1G before 1C) and — if strict FK enforcement
// is required at the DB level — adding an `ALTER TABLE ... ADD
// CONSTRAINT` after both tranches have migrated. See TestRAGSearchSettings_EmbeddingModelFK.
func AutoMigrate1C(db *gorm.DB) error {
	// Phase 3A: RAGConnectionConfig is retired. Its scheduling fields
	// (RefreshFreqSeconds / PruneFreqSeconds / PermSyncFreqSeconds) moved
	// onto RAGSource; its IngestConfig JSONB moved onto RAGSource.Config.
	// AutoMigrate3A drops the rag_connection_configs table entirely for
	// DBs that still have it.
	if err := db.AutoMigrate(
		&RAGSyncState{},
		&RAGSearchSettings{},
	); err != nil {
		return err
	}

	// Composite index for org-wide status scans: admin dashboards
	// ("show me all paused sources in org X"), scheduler gates
	// ("next perm-sync candidate per org"). The column-level indexes
	// gorm auto-creates from the tags aren't enough because we need
	// the (org_id, status) tuple leading.
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_rag_sync_state_org_status
		ON rag_sync_states (org_id, status)`).Error; err != nil {
		return err
	}

	// rag_source_id FK on rag_sync_states is installed by AutoMigrate3A
	// (runs after rag_sources exists).

	// Org FK: every RAG row must cascade from org deletion for GDPR.
	if err := ensureFK(db,
		"rag_sync_states", "fk_rag_sync_state_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_search_settings", "fk_rag_search_settings_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}

	// Cross-tranche FK to rag_embedding_models (owned by 1G). Only
	// added if the target table already exists at this call site —
	// otherwise we silently defer and rely on 1F to re-run migration
	// after 1G has created the table. This makes AutoMigrate1C
	// runnable in isolation for tranche-owned tests that do not need
	// the FK.
	exists, err := tableExists(db, "rag_embedding_models")
	if err != nil {
		return err
	}
	if exists {
		if err := ensureFK(db,
			"rag_search_settings", "fk_rag_search_settings_embedding_model",
			"embedding_model_id", "rag_embedding_models", "id", "RESTRICT",
		); err != nil {
			return err
		}
	}

	return nil
}

// ensureFK adds a FK constraint idempotently. AutoMigrate1C re-runs on
// every boot (CI and dev) — adding the same constraint twice must be a
// no-op.
func ensureFK(
	db *gorm.DB,
	table, constraintName, fkCol, refTable, refCol, onDelete string,
) error {
	// Skip if constraint already present.
	var count int64
	if err := db.Raw(`
		SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE constraint_name = ?
		  AND table_name = ?
		  AND constraint_type = 'FOREIGN KEY'
	`, constraintName, table).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	stmt := `ALTER TABLE ` + table +
		` ADD CONSTRAINT ` + constraintName +
		` FOREIGN KEY (` + fkCol + `)` +
		` REFERENCES ` + refTable + `(` + refCol + `)` +
		` ON DELETE ` + onDelete
	return db.Exec(stmt).Error
}

// tableExists — information_schema probe for cross-tranche ordering
// checks.
func tableExists(db *gorm.DB, name string) (bool, error) {
	var count int64
	if err := db.Raw(`
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = ?
	`, name).Scan(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
