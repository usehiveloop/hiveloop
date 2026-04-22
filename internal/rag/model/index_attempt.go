package model

import (
	"time"

	"github.com/google/uuid"
)

// RAGIndexAttempt represents one attempted ingestion pass of documents
// from a connected source (think: one Google Drive pull, one GitHub
// crawl). Each attempt is scoped to a single (InConnection,
// EmbeddingModel) pair because the one-model-per-org invariant means
// switching the model = starting a new attempt against a new dataset.
//
// Direct port of Onyx `IndexAttempt` at
// backend/onyx/db/models.py:2189-2343.
//
// DEVIATIONS vs Onyx:
//   - PK is a uuid (not an autoincrement int). Hiveloop convention —
//     every table uses uuid.UUID PKs. Zero semantic impact.
//   - `connector_credential_pair_id` becomes `in_connection_id` because
//     Hiveloop has no CCPair; identity lives on InConnection. See the
//     "ARCHITECTURAL NOTE" in plans/onyx-port.md under Tranche 1C.
//   - `search_settings_id` becomes `embedding_model_id` → FK to
//     `rag_embedding_models(id)`. The rag_embedding_models table is
//     owned by Tranche 1G and may not exist yet when this tranche is
//     merged in isolation; the gorm FK declaration is intentionally a
//     string reference rather than a Go struct reference so the Go
//     build stays clean. Tranche 1F (finalizer) runs AFTER 1G and
//     validates the FK resolves. If 1G lands before 1F, all tests
//     pass. If this tranche is merged before 1G, the runtime
//     AutoMigrate will still succeed under gorm (constraint is best-
//     effort under MySQL and is created via a separate DDL under
//     Postgres). See AutoMigrate1B for the explicit guard.
//   - OrgID column added; Onyx uses schema-per-tenant, Hiveloop uses
//     row-level org_id with a CASCADE FK to orgs(id).
type RAGIndexAttempt struct {
	// ID — Onyx models.py:2198
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	// OrgID — Hiveloop addition for row-level tenancy.
	OrgID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`

	// RAGSourceID — adapts Onyx `connector_credential_pair_id`
	// (models.py:2200-2204). FK to rag_sources(id) with CASCADE so that
	// tearing down a source wipes its index-attempt history.
	//
	// Phase 3A swap (was InConnectionID): keyed off RAGSource now,
	// which itself carries the InConnection reference for
	// INTEGRATION-kind sources.
	RAGSourceID uuid.UUID `gorm:"type:uuid;not null;index"`

	// EmbeddingModelID — adapts Onyx `search_settings_id`
	// (models.py:2221-2225). See DEVIATIONS note above regarding 1G
	// cross-tranche ordering.
	EmbeddingModelID *string `gorm:"type:text"`

	// FromBeginning — Onyx models.py:2206-2209. Only set when the
	// attempt was kicked off via the run-once API with reindex-from-
	// zero semantics.
	FromBeginning bool `gorm:"not null;default:false"`

	// Status — Onyx models.py:2210-2212.
	Status IndexingStatus `gorm:"type:text;not null;index"`

	// Document counters — Onyx models.py:2214-2216. Nullable because
	// an attempt that hasn't started reporting progress yet has no
	// meaningful value (a 0 would be indistinguishable from "zero docs
	// processed so far").
	NewDocsIndexed       *int `gorm:"default:0"`
	TotalDocsIndexed     *int `gorm:"default:0"`
	DocsRemovedFromIndex *int `gorm:"default:0"`

	// ErrorMsg / FullExceptionTrace — only populated when Status=failed.
	// Onyx models.py:2217-2220.
	ErrorMsg            *string `gorm:"type:text"`
	FullExceptionTrace  *string `gorm:"type:text"`

	// PollRangeStart / PollRangeEnd — for polling connectors, the
	// window this attempt is fetching. Onyx models.py:2227-2234.
	PollRangeStart *time.Time
	PollRangeEnd   *time.Time

	// CheckpointPointer — key into the RAG filestore where the
	// in-progress checkpoint blob lives; lets us resume a crashed
	// docfetching run. Onyx models.py:2236-2238.
	CheckpointPointer *string `gorm:"type:text"`

	// Coordination fields (replacing Onyx's Redis fencing mechanism
	// with plain Postgres rows). Onyx models.py:2240-2242.
	CeleryTaskID          *string `gorm:"type:text"`
	CancellationRequested bool    `gorm:"not null;default:false"`

	// Batch coordination. Onyx models.py:2244-2251.
	//
	// TotalBatches is set once docfetching finishes enumerating work.
	// IsCoordinationComplete() below keys off nil vs populated here.
	TotalBatches            *int `gorm:""`
	CompletedBatches        int  `gorm:"not null;default:0"`
	TotalFailuresBatchLevel int  `gorm:"not null;default:0"`
	TotalChunks             int  `gorm:"not null;default:0"`

	// Stall detection / heartbeat — Onyx models.py:2253-2264. The
	// watchdog scans `status='in_progress' AND last_progress_time <
	// NOW() - interval` which is why we add a partial index on those
	// two columns (see AutoMigrate1B).
	LastProgressTime          *time.Time
	LastBatchesCompletedCount int `gorm:"not null;default:0"`
	HeartbeatCounter          int `gorm:"not null;default:0"`
	LastHeartbeatValue        int `gorm:"not null;default:0"`
	LastHeartbeatTime         *time.Time

	// Timestamps. Onyx models.py:2266-2280.
	TimeCreated time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP;index"`
	TimeStarted *time.Time
	TimeUpdated time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP;autoUpdateTime"`
}

// TableName pins the Postgres table name. All RAG tables use the
// rag_ prefix for namespace isolation from the core Hiveloop schema.
func (RAGIndexAttempt) TableName() string { return "rag_index_attempts" }

// IsFinished is the Go port of Onyx's `is_finished` method at
// backend/onyx/db/models.py:2334-2335. Proxies to IndexingStatus's
// IsTerminal so the two cannot drift out of sync.
func (a *RAGIndexAttempt) IsFinished() bool {
	return a.Status.IsTerminal()
}

// IsCoordinationComplete returns true once every batch the
// docfetcher enumerated has been fully processed downstream. Port of
// Onyx `is_coordination_complete` at
// backend/onyx/db/models.py:2337-2342.
//
// Subtle but load-bearing: if TotalBatches is nil the answer is
// always false — docfetching hasn't finished enumerating work yet, so
// we can't possibly have processed "all" of it. CompletedBatches can
// legally exceed TotalBatches on rare races (docfetcher finalised the
// count after a processor already incremented), which is why the
// comparison is >= and not ==.
func (a *RAGIndexAttempt) IsCoordinationComplete() bool {
	if a.TotalBatches == nil {
		return false
	}
	return a.CompletedBatches >= *a.TotalBatches
}
