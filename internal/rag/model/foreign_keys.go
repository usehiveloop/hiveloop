package model

import "gorm.io/gorm"

func createForeignKeys(db *gorm.DB) error {
	if err := ensureFK(db,
		"rag_index_attempts", "fk_rag_index_attempts_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_index_attempts", "fk_rag_index_attempts_rag_source",
		"rag_source_id", "rag_sources", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_index_attempt_errors", "fk_rag_index_attempt_errors_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_index_attempt_errors", "fk_rag_index_attempt_errors_rag_source",
		"rag_source_id", "rag_sources", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_sync_records", "fk_rag_sync_records_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_sync_states", "fk_rag_sync_state_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_sync_states", "fk_rag_sync_state_rag_source",
		"rag_source_id", "rag_sources", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_search_settings", "fk_rag_search_settings_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}
	// RESTRICT: retiring an embedding model that active settings still
	// reference should fail loudly rather than orphan rows.
	if err := ensureFK(db,
		"rag_search_settings", "fk_rag_search_settings_embedding_model",
		"embedding_model_id", "rag_embedding_models", "id", "RESTRICT",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_external_user_groups", "fk_rag_external_user_groups_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_external_user_groups", "fk_rag_external_user_groups_source",
		"rag_source_id", "rag_sources", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_user_external_user_groups", "fk_rag_user_external_user_groups_user",
		"user_id", "users", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_user_external_user_groups", "fk_rag_user_external_user_groups_source",
		"rag_source_id", "rag_sources", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_public_external_user_groups", "fk_rag_public_external_user_groups_source",
		"rag_source_id", "rag_sources", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_external_identities", "fk_rag_external_identities_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_external_identities", "fk_rag_external_identities_user",
		"user_id", "users", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_external_identities", "fk_rag_external_identities_source",
		"rag_source_id", "rag_sources", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_sources", "fk_rag_sources_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_sources", "fk_rag_sources_in_connection",
		"in_connection_id", "in_connections", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_sources", "fk_rag_sources_creator",
		"creator_id", "users", "id", "SET NULL",
	); err != nil {
		return err
	}
	return nil
}
