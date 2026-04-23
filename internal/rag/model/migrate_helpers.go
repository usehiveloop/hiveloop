package model

import "gorm.io/gorm"

// ensureFK adds a foreign-key constraint to an existing table if that
// constraint is not already present. Safe to call on every boot; the
// information_schema probe short-circuits the ALTER TABLE when the
// constraint already exists.
func ensureFK(
	db *gorm.DB,
	table, constraintName, fkCol, refTable, refCol, onDelete string,
) error {
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

// ensureCheck adds a CHECK constraint idempotently. Used for
// cross-column invariants that gorm's struct tags can't express (e.g.
// "field A must be null iff field B is 'FOO'").
func ensureCheck(db *gorm.DB, table, name, expr string) error {
	var count int64
	if err := db.Raw(`
		SELECT COUNT(*) FROM information_schema.table_constraints
		WHERE table_name = ? AND constraint_name = ? AND constraint_type = 'CHECK'
	`, table, name).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return db.Exec(
		"ALTER TABLE " + table + " ADD CONSTRAINT " + name + " CHECK (" + expr + ")",
	).Error
}
