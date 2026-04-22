package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// RAGHierarchyNode represents a structural node in a connected source's
// hierarchy (folder, space, channel, project, shared drive, ...). Port of
// Onyx's HierarchyNode at backend/onyx/db/models.py:839-936. Hierarchy
// nodes carry the same permission shape as documents
// (external_user_emails / external_user_group_ids / is_public) so the UI
// can browse the tree under a user's view.
//
// Key deviations from Onyx:
//   - Onyx uses schema-per-tenant; Hiveloop scopes everything by an
//     explicit OrgID column indexed on org_id.
//   - Onyx uses an Integer PK; we keep that Integer shape (int64) because
//     RAGDocument.ParentHierarchyNodeID and the self-referential parent
//     FK both point at it.
//   - Foreign keys are declared as bare column fields WITHOUT gorm
//     association structs (no `Org model.Org` field). This is a
//     deliberate, load-bearing decision: internal/model/org.go imports
//     internal/rag, so pulling model types into the rag package would
//     create an import cycle. See the Phase 0 report and plan section
//     "CRITICAL CONTEXT".
type RAGHierarchyNode struct {
	// Integer PK — mirrors Onyx's `id: Integer, primary_key=True`
	// (models.py:855). We use int64 so there is headroom; Onyx's Python
	// int promotes freely, Postgres BIGSERIAL is a safe match.
	ID int64 `gorm:"primaryKey;autoIncrement"`

	// OrgID is the Hiveloop org_id scope. DEVIATION from Onyx — in Onyx
	// this table lives in a per-tenant schema.
	OrgID uuid.UUID `gorm:"type:uuid;not null;index:idx_rag_hierarchy_node_org"`

	// RawNodeID is the upstream identifier from the source system. E.g.
	// a Google Drive folder ID. For SOURCE nodes it is the source name.
	// Port of `raw_node_id` at models.py:860.
	RawNodeID string `gorm:"not null"`

	// DisplayName is the human label shown in the UI. Port of
	// `display_name` at models.py:864.
	DisplayName string `gorm:"not null"`

	// Link is the deep link back into the source system. Port of `link`
	// at models.py:867 (nullable).
	Link *string

	// Source is the upstream source enum. Port of `source` at
	// models.py:870-872.
	Source DocumentSource `gorm:"type:text;not null;index:idx_rag_hierarchy_node_source_type,priority:1"`

	// NodeType is the structural category (folder / space / channel /
	// ...). Port of `node_type` at models.py:875-877.
	NodeType HierarchyNodeType `gorm:"type:text;not null;index:idx_rag_hierarchy_node_source_type,priority:2"`

	// ExternalUserEmails — permission sync: email addresses of external
	// users with access. Port of models.py:881-883 (nullable
	// postgresql.ARRAY(String)).
	ExternalUserEmails pq.StringArray `gorm:"type:text[]"`

	// ExternalUserGroupIDs — permission sync: source-prefixed group ids.
	// Port of models.py:885-887.
	ExternalUserGroupIDs pq.StringArray `gorm:"type:text[]"`

	// IsPublic indicates whether the node is accessible org-wide or
	// world-public. Port of `is_public` at models.py:890. SOURCE nodes
	// are always public (enforced at ingest time, not by this struct).
	IsPublic bool `gorm:"not null;default:false"`

	// DocumentID is optional — some hierarchy nodes ARE documents
	// (e.g. Confluence pages). Port of `document_id` at models.py:896-898
	// with ondelete=SET NULL. The string type matches RAGDocument.ID.
	DocumentID *string `gorm:"index"`

	// ParentID is the self-referential FK to another RAGHierarchyNode.
	// Port of `parent_id` at models.py:902-904 with ondelete=SET NULL
	// (orphan children are swept by the prune loop, not the DB).
	ParentID *int64 `gorm:"index"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (RAGHierarchyNode) TableName() string { return "rag_hierarchy_nodes" }
