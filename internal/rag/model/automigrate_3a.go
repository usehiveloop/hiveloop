package model

import (
	"gorm.io/gorm"
)

// AutoMigrate3A performs the Phase 3A schema pivot: every RAG table now
// keys off the top-level RAGSource, not the underlying InConnection.
//
// DESTRUCTIVE BY DESIGN. Phase 3 ships before any production data exists
// (the plan explicitly locks this in at `plans/onyx-port-phase3.md` §
// "Migration stance"), so we run pure DDL — no SELECT-UPDATE-INSERT
// backfill, no data-preservation loops. Tables with shape changes are
// created from scratch via gorm.AutoMigrate over the post-3A struct;
// deprecated columns are dropped with `ALTER TABLE ... DROP COLUMN IF
// EXISTS`.
//
// Steps (order matters for FK resolution):
//  1. Seed `supports_rag_source` on `in_integrations`.
//  2. Create `rag_sources` + its CHECK constraint, unique partial
//     index, and performance indexes.
//  3. For every Phase 1 table that previously had `in_connection_id`,
//     drop the old column (no-op if AutoMigrate1* already added
//     `rag_source_id` from the updated struct).
//  4. Rename the doc/hierarchy junction tables from the old
//     `*_by_connections` to `*_by_sources` (no-op on fresh DBs where
//     AutoMigrate1A already created the new names from scratch).
//  5. Drop the retired `rag_connection_configs` table.
//  6. Install the rag_source_id FKs that depended on `rag_sources`
//     existing.
//
// Idempotent: every DDL uses IF NOT EXISTS or IF EXISTS guards.
// Re-running produces no diff. Called from rag.AutoMigrate after every
// Phase 1 tranche's AutoMigrate<N> has landed its table shapes.
func AutoMigrate3A(db *gorm.DB) error {
	// --- 1. supports_rag_source flag on in_integrations ---
	// gorm picks up the new struct field automatically; we only need
	// to seed the Phase 3 allowlist.
	seed := `
		UPDATE in_integrations
		SET supports_rag_source = true
		WHERE provider IN ('github','notion','linear','jira','confluence','slack','google_drive')
		  AND supports_rag_source = false
	`
	if err := db.Exec(seed).Error; err != nil {
		return err
	}

	// --- 2. Create rag_sources table ---
	if err := db.AutoMigrate(&RAGSource{}); err != nil {
		return err
	}

	// FK: rag_sources.org_id → orgs.id CASCADE.
	if err := ensureFK(db,
		"rag_sources", "fk_rag_sources_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}

	// FK: rag_sources.in_connection_id → in_connections.id CASCADE
	// (nullable — only INTEGRATION-kind sources populate this column).
	if err := ensureFK(db,
		"rag_sources", "fk_rag_sources_in_connection",
		"in_connection_id", "in_connections", "id", "CASCADE",
	); err != nil {
		return err
	}

	// FK: rag_sources.creator_id → users.id SET NULL. Deleting a user
	// must not cascade into tombstoning org-owned sources.
	if err := ensureFK(db,
		"rag_sources", "fk_rag_sources_creator",
		"creator_id", "users", "id", "SET NULL",
	); err != nil {
		return err
	}

	// CHECK: INTEGRATION kind requires in_connection_id; non-INTEGRATION
	// must have it null.
	if err := addCheckIfMissing(db,
		"rag_sources", "ck_rag_sources_integration_requires_in_connection",
		`((kind = 'INTEGRATION' AND in_connection_id IS NOT NULL) OR
		  (kind <> 'INTEGRATION' AND in_connection_id IS NULL))`,
	); err != nil {
		return err
	}

	// Unique partial index — at most one RAGSource per InConnection,
	// scoped to INTEGRATION-kind rows (WEBSITE/FILE_UPLOAD have
	// in_connection_id IS NULL and shouldn't collide).
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_rag_sources_in_connection
		ON rag_sources (in_connection_id)
		WHERE kind = 'INTEGRATION'
	`).Error; err != nil {
		return err
	}

	// Admin-dashboard + scheduler-scan indexes.
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_rag_sources_org_status
		ON rag_sources (org_id, status)
	`).Error; err != nil {
		return err
	}

	// Scheduler "which sources need ingest?" gate — partial index keyed
	// on org so the per-org scan touches at most the live rows.
	// Matches the WHERE clause the scheduler uses in Tranche 3C
	// (see plans/onyx-port-phase3.md §Tranche 3C scheduler loop).
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_rag_sources_needs_ingest
		ON rag_sources (org_id)
		WHERE enabled = true AND status IN ('ACTIVE','INITIAL_INDEXING')
	`).Error; err != nil {
		return err
	}

	// Prune-loop index — surface stale sources quickly.
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_rag_sources_last_pruned
		ON rag_sources (last_pruned)
		WHERE last_pruned IS NOT NULL
	`).Error; err != nil {
		return err
	}

	// --- 3. Drop deprecated in_connection_id columns ---
	// Safe because Phase 3 has no production data — the Phase 1 tranches
	// migrate the replacement rag_source_id columns via AutoMigrate over
	// the updated struct; the old columns and their FKs become orphans.
	dropOldColumns := []string{
		// Drop FKs first so the column drop doesn't fail on
		// dependent constraints.
		`ALTER TABLE rag_sync_states DROP CONSTRAINT IF EXISTS fk_rag_sync_state_in_connection`,
		`ALTER TABLE rag_sync_states DROP COLUMN IF EXISTS in_connection_id`,

		`ALTER TABLE rag_index_attempts DROP CONSTRAINT IF EXISTS fk_rag_index_attempts_in_connection`,
		`ALTER TABLE rag_index_attempts DROP COLUMN IF EXISTS in_connection_id`,

		`ALTER TABLE rag_index_attempt_errors DROP CONSTRAINT IF EXISTS fk_rag_index_attempt_errors_in_connection`,
		`ALTER TABLE rag_index_attempt_errors DROP COLUMN IF EXISTS in_connection_id`,

		`ALTER TABLE rag_external_user_groups DROP CONSTRAINT IF EXISTS fk_rag_external_user_groups_conn`,
		`ALTER TABLE rag_external_user_groups DROP COLUMN IF EXISTS in_connection_id`,

		`ALTER TABLE rag_user_external_user_groups DROP CONSTRAINT IF EXISTS fk_rag_user_external_user_groups_conn`,
		`ALTER TABLE rag_user_external_user_groups DROP COLUMN IF EXISTS in_connection_id`,

		`ALTER TABLE rag_public_external_user_groups DROP CONSTRAINT IF EXISTS fk_rag_public_external_user_groups_conn`,
		`ALTER TABLE rag_public_external_user_groups DROP COLUMN IF EXISTS in_connection_id`,

		`ALTER TABLE rag_external_identities DROP CONSTRAINT IF EXISTS fk_rag_external_identities_connection`,
		`ALTER TABLE rag_external_identities DROP COLUMN IF EXISTS in_connection_id`,

		// Stale-sweep indexes keyed on the old column; their
		// replacements were created in AutoMigrate1D. The
		// 1D-era ones (idx_rag_user_external_group_conn_stale,
		// idx_rag_public_external_group_conn_stale) dangle
		// after the column drop; clean them up.
		`DROP INDEX IF EXISTS idx_rag_user_external_group_conn_stale`,
		`DROP INDEX IF EXISTS idx_rag_public_external_group_conn_stale`,

		// Similarly for index-attempt indexes that referenced
		// the old column.
		`DROP INDEX IF EXISTS idx_rag_index_attempt_latest_for_conn`,
		`DROP INDEX IF EXISTS idx_rag_index_attempt_conn_model_updated`,
		`DROP INDEX IF EXISTS idx_rag_index_attempt_conn_model_poll`,

		// sync-state old unique index.
		`DROP INDEX IF EXISTS uq_rag_sync_state_in_connection_id`,

		// external-identity old unique + index.
		`DROP INDEX IF EXISTS uq_rag_external_identity_user_conn`,
		`DROP INDEX IF EXISTS idx_rag_external_identity_conn`,

		// external-user-group old uniqueness.
		`DROP INDEX IF EXISTS uq_rag_external_user_group_conn_ext`,
	}
	for _, ddl := range dropOldColumns {
		if err := db.Exec(ddl).Error; err != nil {
			return err
		}
	}

	// --- 4. Rename legacy junction tables if they still exist ---
	// AutoMigrate1A on a fresh DB will have created the new names
	// directly; this branch exists for DBs migrated under pre-3A code.
	renames := []struct {
		from, to string
	}{
		{"rag_document_by_connections", "rag_document_by_sources"},
		{"rag_hierarchy_node_by_connections", "rag_hierarchy_node_by_sources"},
	}
	for _, r := range renames {
		fromExists, err := tableExists(db, r.from)
		if err != nil {
			return err
		}
		toExists, err := tableExists(db, r.to)
		if err != nil {
			return err
		}
		if fromExists && !toExists {
			if err := db.Exec(
				"ALTER TABLE " + r.from + " RENAME TO " + r.to,
			).Error; err != nil {
				return err
			}
		}
	}

	// Post-rename: the junction column may still be called
	// in_connection_id from the pre-3A shape. Swap to rag_source_id.
	junctionColumnFixups := []string{
		`ALTER TABLE rag_document_by_sources DROP CONSTRAINT IF EXISTS fk_rag_doc_by_conn_connection`,
		`ALTER TABLE rag_document_by_sources DROP COLUMN IF EXISTS in_connection_id`,
		`ALTER TABLE rag_hierarchy_node_by_sources DROP CONSTRAINT IF EXISTS fk_rag_hier_by_conn_connection`,
		`ALTER TABLE rag_hierarchy_node_by_sources DROP COLUMN IF EXISTS in_connection_id`,
		`DROP INDEX IF EXISTS idx_rag_doc_conn_connection`,
		`DROP INDEX IF EXISTS idx_rag_doc_conn_counts`,
		`DROP INDEX IF EXISTS idx_rag_hier_conn_connection`,
	}
	for _, ddl := range junctionColumnFixups {
		if err := db.Exec(ddl).Error; err != nil {
			return err
		}
	}

	// Re-run AutoMigrate over the junction structs so gorm adds the
	// rag_source_id column + its index on tables that were renamed
	// from the old name and never got it.
	if err := db.AutoMigrate(&RAGDocumentBySource{}, &RAGHierarchyNodeBySource{}); err != nil {
		return err
	}

	// --- 5. Drop retired rag_connection_configs ---
	if err := db.Exec(`DROP TABLE IF EXISTS rag_connection_configs CASCADE`).Error; err != nil {
		return err
	}

	// --- 6. Install rag_source_id FKs now that rag_sources exists ---
	sourceFKs := []struct {
		table, name string
	}{
		{"rag_sync_states", "fk_rag_sync_state_rag_source"},
		{"rag_index_attempts", "fk_rag_index_attempts_rag_source"},
		{"rag_index_attempt_errors", "fk_rag_index_attempt_errors_rag_source"},
		{"rag_external_user_groups", "fk_rag_external_user_groups_source"},
		{"rag_user_external_user_groups", "fk_rag_user_external_user_groups_source"},
		{"rag_public_external_user_groups", "fk_rag_public_external_user_groups_source"},
		{"rag_external_identities", "fk_rag_external_identities_source"},
		{"rag_document_by_sources", "fk_rag_doc_by_source_source"},
		{"rag_hierarchy_node_by_sources", "fk_rag_hier_by_source_source"},
	}
	for _, fk := range sourceFKs {
		if err := ensureFK(db,
			fk.table, fk.name,
			"rag_source_id", "rag_sources", "id", "CASCADE",
		); err != nil {
			return err
		}
	}

	return nil
}

// addCheckIfMissing installs a CHECK constraint only if it isn't
// already there. Reads from information_schema.table_constraints so
// it's safe across Postgres versions and idempotent across re-runs.
func addCheckIfMissing(db *gorm.DB, table, name, expr string) error {
	var count int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM information_schema.table_constraints
		 WHERE table_name = ? AND constraint_name = ? AND constraint_type = 'CHECK'`,
		table, name,
	).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return db.Exec(
		"ALTER TABLE " + table + " ADD CONSTRAINT " + name + " CHECK (" + expr + ")",
	).Error
}
