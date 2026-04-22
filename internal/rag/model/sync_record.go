package model

import (
	"time"

	"github.com/google/uuid"
)

// RAGSyncRecord is an audit/status row for a "sync" operation — a
// batch job that reconciles a chunk of Postgres state into the vector
// store (deletion-sweep, pruning sweep, external-permissions refresh,
// external-group refresh).
//
// Subset port of Onyx `SyncRecord` at
// backend/onyx/db/models.py:2440-2478.
//
// DEVIATION vs Onyx:
//   - We exclude the `document_set` and `user_group` SyncType values
//     because Hiveloop has no DocumentSet and no UserGroup concept in
//     Phase 1. See SyncType in enums_index_attempt.go.
//   - OrgID column added; Onyx uses schema-per-tenant, Hiveloop uses
//     row-level tenancy.
//   - PK is a uuid, not an autoincrement int (Hiveloop convention).
type RAGSyncRecord struct {
	// ID — Onyx models.py:2450.
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	// OrgID — Hiveloop addition. Every sync runs inside a single org;
	// the scheduler fans out per-org.
	OrgID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`

	// EntityID — the subject of the sync. Onyx models.py:2452 stored
	// an int (document_set_id / user_group_id / …); we store a uuid
	// because every Hiveloop entity is uuid-keyed. Interpretation is
	// driven by SyncType: for `connector_deletion` / `pruning` /
	// `external_permissions` / `external_group` the EntityID refers to
	// an InConnection.
	EntityID uuid.UUID `gorm:"type:uuid;not null"`

	// SyncType — Onyx models.py:2454. See SyncType in
	// enums_index_attempt.go for the Hiveloop subset.
	SyncType SyncType `gorm:"type:text;not null"`

	// SyncStatus — Onyx models.py:2455.
	SyncStatus SyncStatus `gorm:"type:text;not null"`

	// NumDocsSynced — count of docs touched by this sync. Onyx
	// models.py:2457.
	NumDocsSynced int `gorm:"not null;default:0"`

	// SyncStartTime / SyncEndTime — wall-clock window.
	// Onyx models.py:2459-2462. End is nullable for still-running
	// syncs.
	SyncStartTime time.Time  `gorm:"not null"`
	SyncEndTime   *time.Time
}

// TableName pins the Postgres table name.
func (RAGSyncRecord) TableName() string { return "rag_sync_records" }
