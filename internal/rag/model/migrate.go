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
	return nil
}
