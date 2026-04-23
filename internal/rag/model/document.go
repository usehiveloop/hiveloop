package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// RAGDocument is the per-document metadata row that drives ACL-aware
// retrieval, incremental sync, hierarchy scoping, and ownership display.
// Port of Onyx's Document model at
// backend/onyx/db/models.py:939-1063.
//
// Skipped fields (deliberately out of scope — revisit when the feature
// lands):
//   - kg_stage, kg_processing_time (knowledge graph, deferred)
//   - relationships to Persona / Tag / DocumentRetrievalFeedback
//
// FK style: bare column fields, NO gorm association structs. Adding a
// `Org model.Org` field here would create an import cycle because
// internal/model/org.go already imports internal/rag. This rule is
// load-bearing across the RAG package.
type RAGDocument struct {
	// ID is the source-given opaque document ID. Primary key. Port of
	// `id: NullFilteredString, primary_key=True` at models.py:945.
	// NullFilteredString in Onyx rejects NUL bytes — we do not mirror
	// that at the Postgres layer; connectors normalize upstream.
	ID string `gorm:"type:text;primaryKey"`

	// OrgID scopes the doc to a tenant. DEVIATION from Onyx, which uses
	// schema-per-tenant. Indexed for org-scan queries and every query
	// that filters by tenant — i.e., all of them.
	OrgID uuid.UUID `gorm:"type:uuid;not null;index:idx_rag_document_org"`

	// FromIngestionAPI is true if the document was pushed via the
	// ingestion API rather than pulled by a connector. Port of
	// `from_ingestion_api` at models.py:946-948.
	FromIngestionAPI bool `gorm:"default:false"`

	// Boost is the administrator's relevance tweak: 0 neutral, positive
	// boost, negative bury. Port of `boost` at models.py:950 (Onyx
	// default DEFAULT_BOOST = 0).
	Boost int `gorm:"not null;default:0"`

	// Hidden suppresses the doc from search results (without deleting
	// its chunks). Port of `hidden` at models.py:951.
	Hidden bool `gorm:"not null;default:false"`

	// SemanticID is the display-friendly title shown in UI results.
	// Port of `semantic_id` at models.py:952.
	SemanticID string `gorm:"type:text;not null"`

	// Link is the deep link back into the source system. Port of `link`
	// at models.py:954 (nullable).
	Link *string `gorm:"type:text"`

	// FileID optionally references a blob in the file store. Port of
	// `file_id` at models.py:955.
	FileID *string `gorm:"type:text"`

	// DocUpdatedAt is the upstream "last modified at source" timestamp
	// the connector reported. Used to skip re-indexing unchanged docs.
	// Port of `doc_updated_at` at models.py:963-965. Onyx has a TODO
	// noting this field is overloaded; we inherit the shape unchanged.
	DocUpdatedAt *time.Time

	// ChunkCount is the number of chunks the doc produced in the vector
	// store. Nullable for historical rows pre-dating the field. Port of
	// `chunk_count` at models.py:969.
	ChunkCount *int

	// LastModified is the server-side time at which any of this doc's
	// relevant metadata last changed (not including LastSynced itself).
	// Drives the `needs_sync` partial index below. Port of
	// `last_modified` at models.py:973-975 (nullable=False, default=now).
	LastModified time.Time `gorm:"not null;default:now();index:idx_rag_document_last_modified"`

	// LastSynced is the server-side time of the last successful write
	// to the vector store. Together with LastModified it feeds the
	// sync-needed partial index. Port of `last_synced` at
	// models.py:978-980.
	LastSynced *time.Time `gorm:"index"`

	// PrimaryOwners is the source-reported list of primary authors or
	// owners (emails / handles). Port of `primary_owners` at
	// models.py:984-986.
	PrimaryOwners pq.StringArray `gorm:"type:text[]"`

	// SecondaryOwners — see above. Port of `secondary_owners` at
	// models.py:987-989.
	SecondaryOwners pq.StringArray `gorm:"type:text[]"`

	// ExternalUserEmails is the permission-sync denormalized list of
	// email addresses allowed to see the doc. GIN-indexed because ACL
	// filters scan it by membership. Port of `external_user_emails` at
	// models.py:994-996.
	ExternalUserEmails pq.StringArray `gorm:"type:text[]"`

	// ExternalUserGroupIDs is the list of group IDs (source-prefixed
	// and lowercased — see acl.BuildExtGroupName) with access. Port of
	// `external_user_group_ids` at models.py:998-1000.
	ExternalUserGroupIDs pq.StringArray `gorm:"type:text[]"`

	// IsPublic marks a doc as readable by anyone in the org. Port of
	// `is_public` at models.py:1001.
	IsPublic bool `gorm:"not null;default:false"`

	// ParentHierarchyNodeID is the FK into rag_hierarchy_nodes.id
	// (folder / space / channel containing this doc). ON DELETE SET
	// NULL — deleting a folder must not delete its docs. Port of
	// `parent_hierarchy_node_id` at models.py:1006-1008.
	ParentHierarchyNodeID *int64 `gorm:"index"`

	// DocMetadata is the flexible source-specific metadata bag. Port
	// of `doc_metadata: JSONB, nullable=True` at models.py:1025-1027.
	DocMetadata model.JSON `gorm:"type:jsonb"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (RAGDocument) TableName() string { return "rag_documents" }
