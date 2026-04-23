package model

import (
	"github.com/google/uuid"
)

// RAGHierarchyNodeBySource tracks which RAGSources reference each
// RAGHierarchyNode. Port of Onyx's
// HierarchyNodeByConnectorCredentialPair at
// backend/onyx/db/models.py:2480-2510.
//
// DEVIATION: Hiveloop collapses Onyx's (connector_id, credential_id)
// to rag_source_id so every junction uniformly references the
// top-level RAGSource.
//
// Onyx comment at models.py:2481-2483 documents the pruning pattern:
// during pruning, stale entries for the current source are removed;
// hierarchy nodes with zero remaining entries across all sources can
// then be deleted.
type RAGHierarchyNodeBySource struct {
	// HierarchyNodeID — part of the composite PK, FK to
	// rag_hierarchy_nodes.id with ON DELETE CASCADE. Port of
	// `hierarchy_node_id` at models.py:2489-2491.
	HierarchyNodeID int64 `gorm:"primaryKey"`

	// RAGSourceID — part of the composite PK, FK to rag_sources.id with
	// ON DELETE CASCADE. Adapts Onyx's composite connector_id +
	// credential_id at models.py:2492-2493.
	RAGSourceID uuid.UUID `gorm:"type:uuid;primaryKey;index:idx_rag_hier_source_source"`
}

func (RAGHierarchyNodeBySource) TableName() string {
	return "rag_hierarchy_node_by_sources"
}
