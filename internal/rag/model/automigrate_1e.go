package model

import "gorm.io/gorm"

// AutoMigrate1E migrates Tranche 1E tables and applies the FK / unique
// constraints that gorm's struct-tag inference cannot express on its own.
//
// Ordering contract (enforced by the 1F finalizer that wires this):
//  1. `model.AutoMigrate(db)` runs first and owns `oauth_accounts` — gorm's
//     AutoMigrate is idempotent and picks up the Tranche 1E column additions
//     (ProviderUserEmail, ProviderUserLogin, VerifiedEmails, LastSyncedAt)
//     from the struct automatically. We do NOT re-migrate OAuthAccount here.
//  2. The other tranches' AutoMigrate helpers run.
//  3. This function runs, migrating `rag_external_identities` and locking in
//     its FK / unique constraints.
//
// Idempotent: every ALTER is guarded by a pg_constraint lookup so re-runs
// (CI, boot-time migrations) don't fail.
func AutoMigrate1E(db *gorm.DB) error {
	if err := db.AutoMigrate(&RAGExternalIdentity{}); err != nil {
		return err
	}

	// FK: org_id → orgs.id ON DELETE CASCADE.
	// We can't use gorm association tags (internal/model/org.go imports
	// internal/rag, so association fields would create an import cycle).
	if err := addFKIfMissing(db,
		"rag_external_identities", "fk_rag_external_identities_org",
		"FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE",
	); err != nil {
		return err
	}

	// FK: user_id → users.id ON DELETE CASCADE.
	if err := addFKIfMissing(db,
		"rag_external_identities", "fk_rag_external_identities_user",
		"FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE",
	); err != nil {
		return err
	}

	// rag_source_id FK installed by AutoMigrate3A once rag_sources exists.

	return nil
}

// addFKIfMissing is a guarded ALTER TABLE ... ADD CONSTRAINT. Reads from
// pg_constraint via information_schema so it works on every supported
// Postgres version and is safe to re-run.
func addFKIfMissing(db *gorm.DB, table, name, spec string) error {
	var count int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM information_schema.table_constraints
         WHERE table_name = ? AND constraint_name = ?`,
		table, name,
	).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return db.Exec(
		"ALTER TABLE " + table + " ADD CONSTRAINT " + name + " " + spec,
	).Error
}
