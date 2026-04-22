package model

import "gorm.io/gorm"

// AutoMigrate1A runs the schema migrations owned by Tranche 1A:
//   - rag_documents, rag_hierarchy_nodes,
//     rag_document_by_connections, rag_hierarchy_node_by_connections
//   - all indexes that cannot be expressed via gorm tags (partial,
//     GIN, FK actions that diverge from gorm's default).
//
// This function is deliberately isolated from internal/rag/register.go
// so Tranche 1F can wire it in alongside every other tranche's
// AutoMigrate<N> without touching code owned by parallel tranches.
// Plan reference: "Tranche 1F" in plans/onyx-port.md.
//
// Must be idempotent. Tests call it directly; Phase 1F will call it
// from rag.AutoMigrate.
func AutoMigrate1A(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&RAGHierarchyNode{},
		&RAGDocument{},
		&RAGDocumentByConnection{},
		&RAGHierarchyNodeByConnection{},
	); err != nil {
		return err
	}

	// FK on RAGDocument.OrgID → orgs.id ON DELETE CASCADE. The plan
	// mandates org-cascade delete for GDPR compliance. gorm's default
	// auto-created FK does not set a delete action; we create it
	// explicitly (dropping any prior version for idempotence).
	stmts := []string{
		// Org cascade on rag_documents
		`ALTER TABLE rag_documents
		   DROP CONSTRAINT IF EXISTS fk_rag_documents_org`,
		`ALTER TABLE rag_documents
		   ADD CONSTRAINT fk_rag_documents_org
		   FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE`,

		// Org cascade on rag_hierarchy_nodes
		`ALTER TABLE rag_hierarchy_nodes
		   DROP CONSTRAINT IF EXISTS fk_rag_hierarchy_nodes_org`,
		`ALTER TABLE rag_hierarchy_nodes
		   ADD CONSTRAINT fk_rag_hierarchy_nodes_org
		   FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE`,

		// RAGDocument.ParentHierarchyNodeID → rag_hierarchy_nodes.id
		// ON DELETE SET NULL — port of Onyx's ondelete="SET NULL" at
		// backend/onyx/db/models.py:1006-1008. Deleting a folder must
		// not delete its docs.
		`ALTER TABLE rag_documents
		   DROP CONSTRAINT IF EXISTS fk_rag_documents_parent_hierarchy_node`,
		`ALTER TABLE rag_documents
		   ADD CONSTRAINT fk_rag_documents_parent_hierarchy_node
		   FOREIGN KEY (parent_hierarchy_node_id)
		   REFERENCES rag_hierarchy_nodes(id)
		   ON DELETE SET NULL`,

		// RAGHierarchyNode self-FK (parent_id) — port of Onyx's
		// ondelete="SET NULL" at models.py:902-904.
		`ALTER TABLE rag_hierarchy_nodes
		   DROP CONSTRAINT IF EXISTS fk_rag_hierarchy_nodes_parent`,
		`ALTER TABLE rag_hierarchy_nodes
		   ADD CONSTRAINT fk_rag_hierarchy_nodes_parent
		   FOREIGN KEY (parent_id)
		   REFERENCES rag_hierarchy_nodes(id)
		   ON DELETE SET NULL`,

		// RAGHierarchyNode.DocumentID → rag_documents.id ON DELETE
		// SET NULL — port of models.py:896-898.
		`ALTER TABLE rag_hierarchy_nodes
		   DROP CONSTRAINT IF EXISTS fk_rag_hierarchy_nodes_document`,
		`ALTER TABLE rag_hierarchy_nodes
		   ADD CONSTRAINT fk_rag_hierarchy_nodes_document
		   FOREIGN KEY (document_id)
		   REFERENCES rag_documents(id)
		   ON DELETE SET NULL`,

		// Junction: RAGDocumentByConnection — both FKs cascade
		// so connection removal sweeps edges (but leaves the doc).
		`ALTER TABLE rag_document_by_connections
		   DROP CONSTRAINT IF EXISTS fk_rag_doc_by_conn_document`,
		`ALTER TABLE rag_document_by_connections
		   ADD CONSTRAINT fk_rag_doc_by_conn_document
		   FOREIGN KEY (document_id)
		   REFERENCES rag_documents(id)
		   ON DELETE CASCADE`,

		`ALTER TABLE rag_document_by_connections
		   DROP CONSTRAINT IF EXISTS fk_rag_doc_by_conn_connection`,
		`ALTER TABLE rag_document_by_connections
		   ADD CONSTRAINT fk_rag_doc_by_conn_connection
		   FOREIGN KEY (in_connection_id)
		   REFERENCES in_connections(id)
		   ON DELETE CASCADE`,

		// Junction: RAGHierarchyNodeByConnection — port of
		// models.py:2489-2493 (both CASCADE).
		`ALTER TABLE rag_hierarchy_node_by_connections
		   DROP CONSTRAINT IF EXISTS fk_rag_hier_by_conn_node`,
		`ALTER TABLE rag_hierarchy_node_by_connections
		   ADD CONSTRAINT fk_rag_hier_by_conn_node
		   FOREIGN KEY (hierarchy_node_id)
		   REFERENCES rag_hierarchy_nodes(id)
		   ON DELETE CASCADE`,

		`ALTER TABLE rag_hierarchy_node_by_connections
		   DROP CONSTRAINT IF EXISTS fk_rag_hier_by_conn_connection`,
		`ALTER TABLE rag_hierarchy_node_by_connections
		   ADD CONSTRAINT fk_rag_hier_by_conn_connection
		   FOREIGN KEY (in_connection_id)
		   REFERENCES in_connections(id)
		   ON DELETE CASCADE`,

		// Partial index driving the sync loop — port of Onyx
		// `ix_document_needs_sync` at models.py:1058-1062.
		`CREATE INDEX IF NOT EXISTS idx_rag_document_needs_sync
		   ON rag_documents (id)
		   WHERE last_modified > last_synced OR last_synced IS NULL`,

		// GIN indexes on the ACL array columns. Hiveloop-explicit —
		// Onyx uses Postgres's default ARRAY indexing; we want GIN
		// because LanceDB mirror filters hit these same columns from
		// perm-sync paths at production scale.
		`CREATE INDEX IF NOT EXISTS idx_rag_document_ext_emails
		   ON rag_documents USING GIN (external_user_emails)`,
		`CREATE INDEX IF NOT EXISTS idx_rag_document_ext_group_ids
		   ON rag_documents USING GIN (external_user_group_ids)`,

		// Unique constraint on (raw_node_id, source) — port of Onyx
		// `uq_hierarchy_node_raw_id_source` at models.py:930-932.
		// Prevents double-indexing the same source page under two
		// different HierarchyNode rows.
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_rag_hierarchy_node_raw_id_source
		   ON rag_hierarchy_nodes (raw_node_id, source)`,
	}

	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			return err
		}
	}

	return nil
}
