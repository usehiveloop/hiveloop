package model

// Enums for the index-attempt + sync-record tables
// (RAGIndexAttempt, RAGIndexAttemptError, RAGSyncRecord).
//
// Onyx sources:
//   - IndexingStatus : backend/onyx/db/enums.py:38-62
//   - IndexingMode   : backend/onyx/db/enums.py:88-91
//   - SyncType       : backend/onyx/db/enums.py:101-111 (subset — see note)
//   - SyncStatus     : backend/onyx/db/enums.py:113-127

// IndexingStatus is the lifecycle status of a single RAGIndexAttempt.
// Verbatim port of Onyx `IndexingStatus` at
// backend/onyx/db/enums.py:38-62 — value strings are byte-identical so
// Postgres-level rows survive a re-indexing port.
type IndexingStatus string

const (
	IndexingStatusNotStarted           IndexingStatus = "not_started"
	IndexingStatusInProgress           IndexingStatus = "in_progress"
	IndexingStatusSuccess              IndexingStatus = "success"
	IndexingStatusCanceled             IndexingStatus = "canceled"
	IndexingStatusFailed               IndexingStatus = "failed"
	IndexingStatusCompletedWithErrors  IndexingStatus = "completed_with_errors"
)

// IsTerminal returns true for every status that the scheduler should
// treat as "finished" (successful or not). Port of Onyx
// `IndexingStatus.is_terminal` at backend/onyx/db/enums.py:46-53.
//
// The scheduler uses this to decide whether it's safe to spawn a new
// attempt for the same (connection, embedding model) pair — a wrong
// answer here stalls the indexing queue, so the branch coverage on
// this function is load-bearing.
func (s IndexingStatus) IsTerminal() bool {
	switch s {
	case IndexingStatusSuccess,
		IndexingStatusCompletedWithErrors,
		IndexingStatusCanceled,
		IndexingStatusFailed:
		return true
	default:
		return false
	}
}

// IsSuccessful returns true when the attempt produced usable index
// output. "completed_with_errors" counts as successful per Onyx's
// convention at backend/onyx/db/enums.py:55-59 — partial failures still
// yield retrievable documents.
func (s IndexingStatus) IsSuccessful() bool {
	return s == IndexingStatusSuccess || s == IndexingStatusCompletedWithErrors
}

// IndexingMode selects full reindex vs incremental update. Verbatim
// port of Onyx `IndexingMode` at backend/onyx/db/enums.py:88-91.
type IndexingMode string

const (
	IndexingModeUpdate  IndexingMode = "update"
	IndexingModeReindex IndexingMode = "reindex"
)

// SyncType is the kind of sync operation represented by a
// RAGSyncRecord. Subset port of Onyx `SyncType` at
// backend/onyx/db/enums.py:101-111.
//
// DEVIATION: we intentionally DO NOT port `document_set` or
// `user_group` — Hiveloop has neither concept.
type SyncType string

const (
	SyncTypeConnectorDeletion   SyncType = "connector_deletion"
	SyncTypePruning             SyncType = "pruning"
	SyncTypeExternalPermissions SyncType = "external_permissions"
	SyncTypeExternalGroup       SyncType = "external_group"
)

// IsValid returns true when s is one of the recognised SyncType values.
// Used by API input validation at the edges of the RAG sync coordinator.
func (s SyncType) IsValid() bool {
	switch s {
	case SyncTypeConnectorDeletion,
		SyncTypePruning,
		SyncTypeExternalPermissions,
		SyncTypeExternalGroup:
		return true
	default:
		return false
	}
}

// SyncStatus is the lifecycle state of a RAGSyncRecord. Verbatim port
// of Onyx `SyncStatus` at backend/onyx/db/enums.py:113-127.
type SyncStatus string

const (
	SyncStatusInProgress SyncStatus = "in_progress"
	SyncStatusSuccess    SyncStatus = "success"
	SyncStatusFailed     SyncStatus = "failed"
	SyncStatusCanceled   SyncStatus = "canceled"
)

// IsTerminal returns true for sync statuses that conclude the sync
// run. Port of Onyx `SyncStatus.is_terminal` at
// backend/onyx/db/enums.py:119-125. Note the subtle difference vs
// IndexingStatus.IsTerminal: `canceled` is NOT terminal for a sync
// (Onyx distinguishes "user cancelled, still cleaning up" from
// "truly done"). Do not "fix" this to match IndexingStatus — it's
// intentional.
func (s SyncStatus) IsTerminal() bool {
	switch s {
	case SyncStatusSuccess, SyncStatusFailed:
		return true
	default:
		return false
	}
}
