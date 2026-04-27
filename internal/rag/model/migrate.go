package model

import "gorm.io/gorm"

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&RAGSource{},
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
