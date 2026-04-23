package model

// Enums for the sync-state + search-settings tables.

// RAGConnectionStatus — port of Onyx `ConnectorCredentialPairStatus` at
// backend/onyx/db/enums.py:180-205.
//
// Values are verbatim (uppercase) to preserve on-disk compatibility with
// any tooling that inspects Onyx dumps; Onyx stores these via
// `Enum(..., native_enum=False)` which writes the enum NAME (uppercase)
// rather than the VALUE to the column.
type RAGConnectionStatus string

const (
	RAGConnectionStatusScheduled       RAGConnectionStatus = "SCHEDULED"
	RAGConnectionStatusInitialIndexing RAGConnectionStatus = "INITIAL_INDEXING"
	RAGConnectionStatusActive          RAGConnectionStatus = "ACTIVE"
	RAGConnectionStatusPaused          RAGConnectionStatus = "PAUSED"
	RAGConnectionStatusDeleting        RAGConnectionStatus = "DELETING"
	RAGConnectionStatusInvalid         RAGConnectionStatus = "INVALID"
)

// ActiveStatuses — port of Onyx `active_statuses` classmethod at
// backend/onyx/db/enums.py:188-194. Returned slice is a fresh copy so
// callers can mutate without polluting subsequent calls.
func ActiveStatuses() []RAGConnectionStatus {
	return []RAGConnectionStatus{
		RAGConnectionStatusActive,
		RAGConnectionStatusScheduled,
		RAGConnectionStatusInitialIndexing,
	}
}

// IndexableStatuses — port of Onyx `indexable_statuses` classmethod at
// backend/onyx/db/enums.py:196-201. Superset of ActiveStatuses including
// PAUSED (per Onyx comment: "Superset of active statuses for indexing
// model swaps").
func IndexableStatuses() []RAGConnectionStatus {
	return append(ActiveStatuses(), RAGConnectionStatusPaused)
}

// IsActive — port of Onyx `is_active` method at
// backend/onyx/db/enums.py:203-204.
func (s RAGConnectionStatus) IsActive() bool {
	for _, v := range ActiveStatuses() {
		if s == v {
			return true
		}
	}
	return false
}

// AccessType — verbatim port of Onyx `AccessType` at
// backend/onyx/db/enums.py:207-211.
type AccessType string

const (
	AccessTypePublic  AccessType = "public"
	AccessTypePrivate AccessType = "private"
	AccessTypeSync    AccessType = "sync"
)

// ProcessingMode — verbatim port of Onyx `ProcessingMode` at
// backend/onyx/db/enums.py:93-98. Controls the docfetching branch
// that selects the post-fetch pipeline (full chunk -> embed -> store
// vs FS drop vs raw binary drop).
type ProcessingMode string

const (
	ProcessingModeRegular    ProcessingMode = "REGULAR"
	ProcessingModeFileSystem ProcessingMode = "FILE_SYSTEM"
	ProcessingModeRawBinary  ProcessingMode = "RAW_BINARY"
)

// EmbeddingPrecision — port of Onyx `EmbeddingPrecision` at
// backend/onyx/db/enums.py:213-241. Vespa tensor-type echo; retained
// verbatim because downstream LanceDB columnar precision maps 1:1.
type EmbeddingPrecision string

const (
	EmbeddingPrecisionBFloat16 EmbeddingPrecision = "bfloat16"
	EmbeddingPrecisionFloat    EmbeddingPrecision = "float"
)
