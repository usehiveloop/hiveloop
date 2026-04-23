package model

import "gorm.io/gorm"

// createForeignKeys installs the foreign-key constraints with explicit
// ON DELETE semantics. gorm's AutoMigrate auto-creates FKs for
// association fields but we use bare column fields (no association
// structs — an association would create an import cycle with
// internal/model), so the FKs need to be declared here.
func createForeignKeys(db *gorm.DB) error {
	// --- Document + hierarchy tree ---

	// Deleting an org wipes its documents. GDPR tenant-deletion
	// invariant.
	if err := ensureFK(db,
		"rag_documents", "fk_rag_documents_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}
	// Deleting an org wipes its hierarchy nodes.
	if err := ensureFK(db,
		"rag_hierarchy_nodes", "fk_rag_hierarchy_nodes_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}
	// Deleting a folder must NOT delete its documents — the docs lose
	// their parent pointer and wait for re-parenting on the next sync.
	// Port of Onyx's ondelete="SET NULL" at
	// backend/onyx/db/models.py:1006-1008.
	if err := ensureFK(db,
		"rag_documents", "fk_rag_documents_parent_hierarchy_node",
		"parent_hierarchy_node_id", "rag_hierarchy_nodes", "id", "SET NULL",
	); err != nil {
		return err
	}
	// Deleting a hierarchy node must NOT delete the underlying
	// document — some hierarchy nodes ARE documents (e.g. Confluence
	// pages) and pruning the tree shouldn't vacuum the page. Port of
	// Onyx's ondelete="SET NULL" at models.py:896-898.
	if err := ensureFK(db,
		"rag_hierarchy_nodes", "fk_rag_hierarchy_nodes_document",
		"document_id", "rag_documents", "id", "SET NULL",
	); err != nil {
		return err
	}
	// Self-referential parent link on the hierarchy tree; orphan
	// children are swept by the prune loop, not the DB. Port of Onyx's
	// ondelete="SET NULL" at models.py:902-904.
	if err := ensureFK(db,
		"rag_hierarchy_nodes", "fk_rag_hierarchy_nodes_parent",
		"parent_id", "rag_hierarchy_nodes", "id", "SET NULL",
	); err != nil {
		return err
	}

	// --- Junction: document <-> source ---

	// Removing a document sweeps its junction rows too (the shared
	// document row itself is only deleted when no source still
	// references it, handled by the prune loop).
	if err := ensureFK(db,
		"rag_document_by_sources", "fk_rag_doc_by_source_document",
		"document_id", "rag_documents", "id", "CASCADE",
	); err != nil {
		return err
	}
	// Removing a source sweeps its indexing edges.
	if err := ensureFK(db,
		"rag_document_by_sources", "fk_rag_doc_by_source_source",
		"rag_source_id", "rag_sources", "id", "CASCADE",
	); err != nil {
		return err
	}

	// --- Junction: hierarchy-node <-> source ---

	if err := ensureFK(db,
		"rag_hierarchy_node_by_sources", "fk_rag_hier_by_source_node",
		"hierarchy_node_id", "rag_hierarchy_nodes", "id", "CASCADE",
	); err != nil {
		return err
	}
	if err := ensureFK(db,
		"rag_hierarchy_node_by_sources", "fk_rag_hier_by_source_source",
		"rag_source_id", "rag_sources", "id", "CASCADE",
	); err != nil {
		return err
	}

	// --- Index attempts + errors + sync records ---

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

	// --- Sync state + search settings ---

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
	// RESTRICT on the embedding-model FK: retiring a model that
	// active search settings still point to should fail loudly rather
	// than orphan rows.
	if err := ensureFK(db,
		"rag_search_settings", "fk_rag_search_settings_embedding_model",
		"embedding_model_id", "rag_embedding_models", "id", "RESTRICT",
	); err != nil {
		return err
	}

	// --- External groups + identity ---

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

	// --- RAGSource itself ---

	if err := ensureFK(db,
		"rag_sources", "fk_rag_sources_org",
		"org_id", "orgs", "id", "CASCADE",
	); err != nil {
		return err
	}
	// Deleting an InConnection cascades into its source — an
	// integration disconnected by the user should sweep the RAG data
	// that depends on it.
	if err := ensureFK(db,
		"rag_sources", "fk_rag_sources_in_connection",
		"in_connection_id", "in_connections", "id", "CASCADE",
	); err != nil {
		return err
	}
	// Deleting a user must NOT delete org-owned sources they happened
	// to create.
	if err := ensureFK(db,
		"rag_sources", "fk_rag_sources_creator",
		"creator_id", "users", "id", "SET NULL",
	); err != nil {
		return err
	}

	return nil
}
