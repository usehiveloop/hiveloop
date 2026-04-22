package model

import (
	"gorm.io/gorm"
)

// AutoMigrate1D migrates the three external-group tables owned by
// tranche 1D and installs their indexes + foreign keys via raw SQL.
//
// It is called from the central tranche-1F wiring at
// internal/rag/register.go:AutoMigrate. It is safe to run multiple
// times — every raw-SQL statement uses `IF NOT EXISTS` or an
// idempotent DDL guard.
//
// Foreign keys are added manually rather than via gorm's `constraint`
// tag because the RAG models deliberately avoid association fields
// (see plan, "universal constraints"). The FKs are real Postgres FK
// constraints, not gorm-only metadata.
func AutoMigrate1D(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&RAGExternalUserGroup{},
		&RAGUserExternalUserGroup{},
		&RAGPublicExternalUserGroup{},
	); err != nil {
		return err
	}

	// ------------------------------------------------------------------
	// Foreign keys (CASCADE on delete) — gorm does not emit these
	// without association fields.
	// ------------------------------------------------------------------

	// RAGExternalUserGroup → orgs, in_connections
	if err := db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_rag_external_user_groups_org') THEN
				ALTER TABLE rag_external_user_groups
				ADD CONSTRAINT fk_rag_external_user_groups_org
				FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_rag_external_user_groups_conn') THEN
				ALTER TABLE rag_external_user_groups
				ADD CONSTRAINT fk_rag_external_user_groups_conn
				FOREIGN KEY (in_connection_id) REFERENCES in_connections(id) ON DELETE CASCADE;
			END IF;
		END$$;
	`).Error; err != nil {
		return err
	}

	// RAGUserExternalUserGroup → users, in_connections
	if err := db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_rag_user_external_user_groups_user') THEN
				ALTER TABLE rag_user_external_user_groups
				ADD CONSTRAINT fk_rag_user_external_user_groups_user
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_rag_user_external_user_groups_conn') THEN
				ALTER TABLE rag_user_external_user_groups
				ADD CONSTRAINT fk_rag_user_external_user_groups_conn
				FOREIGN KEY (in_connection_id) REFERENCES in_connections(id) ON DELETE CASCADE;
			END IF;
		END$$;
	`).Error; err != nil {
		return err
	}

	// RAGPublicExternalUserGroup → in_connections
	if err := db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_rag_public_external_user_groups_conn') THEN
				ALTER TABLE rag_public_external_user_groups
				ADD CONSTRAINT fk_rag_public_external_user_groups_conn
				FOREIGN KEY (in_connection_id) REFERENCES in_connections(id) ON DELETE CASCADE;
			END IF;
		END$$;
	`).Error; err != nil {
		return err
	}

	// ------------------------------------------------------------------
	// Secondary indexes — ports of the Onyx index names, Hiveloop-style
	// `idx_rag_*` naming convention.
	// ------------------------------------------------------------------

	// Port of `ix_user_external_group_cc_pair_stale` (models.py:4340-4344).
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_rag_user_external_group_conn_stale
		ON rag_user_external_user_groups (in_connection_id, stale)`).Error; err != nil {
		return err
	}
	// Port of `ix_user_external_group_stale` (models.py:4345-4348).
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_rag_user_external_group_stale
		ON rag_user_external_user_groups (stale)`).Error; err != nil {
		return err
	}

	// Port of `ix_public_external_group_cc_pair_stale` (models.py:4371-4375).
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_rag_public_external_group_conn_stale
		ON rag_public_external_user_groups (in_connection_id, stale)`).Error; err != nil {
		return err
	}
	// Port of `ix_public_external_group_stale` (models.py:4376-4379).
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_rag_public_external_group_stale
		ON rag_public_external_user_groups (stale)`).Error; err != nil {
		return err
	}

	return nil
}
