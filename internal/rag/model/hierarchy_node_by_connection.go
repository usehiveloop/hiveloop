package model

import (
	"github.com/google/uuid"
)

// RAGHierarchyNodeByConnection tracks which InConnections reference
// each RAGHierarchyNode. Port of Onyx's
// HierarchyNodeByConnectorCredentialPair at
// backend/onyx/db/models.py:2480-2510.
//
// DEVIATION: same (connector_id, credential_id) → in_connection_id
// collapse as RAGDocumentByConnection.
//
// Onyx comment at models.py:2481-2483 documents the pruning pattern:
// during pruning, stale entries for the current connection are removed;
// hierarchy nodes with zero remaining entries across all connections
// can then be deleted.
type RAGHierarchyNodeByConnection struct {
	// HierarchyNodeID — part of the composite PK, FK to
	// rag_hierarchy_nodes.id with ON DELETE CASCADE. Port of
	// `hierarchy_node_id` at models.py:2489-2491.
	HierarchyNodeID int64 `gorm:"primaryKey"`

	// InConnectionID — part of the composite PK, FK to
	// in_connections.id with ON DELETE CASCADE. Adapts Onyx's composite
	// connector_id + credential_id at models.py:2492-2493.
	InConnectionID uuid.UUID `gorm:"type:uuid;primaryKey;index:idx_rag_hier_conn_connection"`
}

func (RAGHierarchyNodeByConnection) TableName() string {
	return "rag_hierarchy_node_by_connections"
}
