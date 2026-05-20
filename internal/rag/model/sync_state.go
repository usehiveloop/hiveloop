package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// RAGSyncState adapts the sync-state subset of Onyx's
// `ConnectorCredentialPair` at backend/onyx/db/models.py:723-837.
//
// ARCHITECTURAL NOTE: Onyx bundles identity + schedule + sync state
// into a single `ConnectorCredentialPair` row. Hivy splits those
// concerns: identity lives on `InConnection`, schedule + config live
// on `RAGSource`, and sync state (this struct) is a 1:1 sibling of
// `RAGSource` keyed by `rag_source_id`. The uniqueness of
// `rag_source_id` is the invariant the three-loop scheduler (see Onyx
// `backend/onyx/background/celery/tasks/{docfetching,docprocessing,
// pruning,doc_permission_syncing,external_group_syncing}`) depends
// on: every loop picks up at most one sync state per source.
//
// Skipped from Onyx 739-800: `connector_id`, `credential_id`, `name`
// (identity columns — live on InConnection).
type RAGSyncState struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	// OrgID — Hivy addition per universal constraint (every RAG row
	// carries org_id with CASCADE). Onyx uses schema-per-tenant.
	OrgID uuid.UUID `gorm:"type:uuid;not null;index"`

	// RAGSourceID — 1:1 link to RAGSource. Unique: a source has at most
	// one sync state. CASCADE mirrors the Onyx behavior where deleting
	// a CCPair zaps its sync metadata. INTEGRATION-kind sources carry
	// the InConnection reference on the RAGSource row itself; WEBSITE
	// / FILE_UPLOAD sources don't have one.
	RAGSourceID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_rag_sync_state_rag_source_id"`

	// Status — port of Onyx models.py:739-741. Drives the
	// scheduler's "should I run?" gate; see RAGConnectionStatus.IsActive.
	Status RAGConnectionStatus `gorm:"type:varchar(32);not null"`

	// InRepeatedErrorState — port of Onyx models.py:744. Orthogonal to
	// Status; a connection can be ACTIVE but flagged as repeatedly
	// failing, which the UI surfaces without pausing the loop.
	InRepeatedErrorState bool `gorm:"not null;default:false"`

	// AccessType — port of Onyx models.py:757-759.
	AccessType AccessType `gorm:"type:varchar(16);not null"`

	// AutoSyncOptions — port of Onyx models.py:766-768 (JSONB). Shape
	// is connector-specific (e.g. `{"customer_id": "...", "company_domain": "..."}`
	// for Google Drive perm sync).
	AutoSyncOptions model.JSON `gorm:"type:jsonb"`

	// LastTimePermSync — port of Onyx models.py:769-771.
	LastTimePermSync *time.Time `gorm:"type:timestamptz"`

	// LastTimeExternalGroupSync — port of Onyx models.py:772-774.
	LastTimeExternalGroupSync *time.Time `gorm:"type:timestamptz"`

	// LastSuccessfulIndexTime — port of Onyx models.py:776-778. Finish
	// time (not start time), per Onyx comment at 775.
	LastSuccessfulIndexTime *time.Time `gorm:"type:timestamptz"`

	// LastPruned — port of Onyx models.py:781-783.
	LastPruned *time.Time `gorm:"type:timestamptz;index:idx_rag_sync_state_last_pruned"`

	// LastTimeHierarchyFetch — port of Onyx models.py:786-788.
	LastTimeHierarchyFetch *time.Time `gorm:"type:timestamptz"`

	// TotalDocsIndexed — port of Onyx models.py:790.
	TotalDocsIndexed int `gorm:"not null;default:0"`

	// IndexingTrigger — port of Onyx models.py:792-794. Nullable: a
	// one-shot signal the API sets to ask the scheduler to do
	// `update` vs `reindex` on the next pass. Typed as *string to
	// avoid the redundant import of IndexingMode at this site; the
	// Postgres column accepts the same values either way.
	IndexingTrigger *string `gorm:"type:varchar(16)"`

	// ProcessingMode — port of Onyx models.py:799-804.
	ProcessingMode ProcessingMode `gorm:"type:varchar(16);not null;default:'REGULAR'"`

	// DeletionFailureMessage — port of Onyx models.py:749.
	DeletionFailureMessage *string `gorm:"type:text"`

	// CreatorID — port of Onyx models.py:827. The user who created
	// the underlying CCPair. Nullable because deletion of the user
	// should not cascade into tombstoning the sync state; the row is
	// owned by the org, not the creator.
	CreatorID *uuid.UUID `gorm:"type:uuid"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName — follow Hivy convention (`orgs`, `in_connections`,
// etc.); RAG tables prefixed `rag_`.
func (RAGSyncState) TableName() string { return "rag_sync_states" }
