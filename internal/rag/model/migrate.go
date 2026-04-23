package model

import "gorm.io/gorm"

// Migrate creates and reconciles every table owned by the RAG model
// package. Idempotent; safe to call multiple times.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&RAGSource{},
		&RAGDocument{},
		&RAGHierarchyNode{},
		&RAGDocumentBySource{},
		&RAGHierarchyNodeBySource{},
		&RAGIndexAttempt{},
		&RAGIndexAttemptError{},
		&RAGSyncRecord{},
		&RAGSyncState{},
		&RAGSearchSettings{},
		&RAGExternalUserGroup{},
		&RAGUserExternalUserGroup{},
		&RAGPublicExternalUserGroup{},
		&RAGExternalIdentity{},
	); err != nil {
		return err
	}

	if err := createIndexes(db); err != nil {
		return err
	}
	if err := createConstraints(db); err != nil {
		return err
	}
	if err := createForeignKeys(db); err != nil {
		return err
	}
	if err := seedIntegrationSupport(db); err != nil {
		return err
	}
	return nil
}

// seedIntegrationSupport flips the supports_rag_source flag on the
// built-in integration providers. The admin UI's "Add RAG source" picker
// only shows integrations with this flag set, so the list is curated
// rather than every integration Hiveloop knows about.
func seedIntegrationSupport(db *gorm.DB) error {
	return db.Exec(`
		UPDATE in_integrations
		SET supports_rag_source = true
		WHERE provider IN ('github','notion','linear','jira','confluence','slack','google_drive')
		  AND supports_rag_source = false
	`).Error
}
