package model

import "gorm.io/gorm"

// createConstraints installs table-level CHECK constraints.
func createConstraints(db *gorm.DB) error {
	// INTEGRATION-kind sources must carry an in_connection_id; every
	// other kind (WEBSITE, FILE_UPLOAD) must have it NULL. Enforcing
	// this at the DB prevents an admin API bug from cross-wiring an
	// integration FK onto a website row.
	if err := ensureCheck(db,
		"rag_sources",
		"ck_rag_sources_integration_requires_in_connection",
		`((kind = 'INTEGRATION' AND in_connection_id IS NOT NULL) OR
		  (kind <> 'INTEGRATION' AND in_connection_id IS NULL))`,
	); err != nil {
		return err
	}
	return nil
}
