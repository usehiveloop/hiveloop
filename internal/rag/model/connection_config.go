package model

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// Validation sentinel errors — port of the two ValueError branches at
// backend/onyx/db/models.py:1919-1929. Exported so API handlers can
// do typed `errors.Is` checks for request-validation responses.
var (
	ErrRefreshFreqTooSmall = errors.New("refresh_freq must be greater than or equal to 1 minute (60 seconds)")
	ErrPruneFreqTooSmall   = errors.New("prune_freq must be greater than or equal to 5 minutes (300 seconds)")
)

// RAGConnectionConfig adapts the scheduling + connector-specific config
// subset of Onyx's `Connector` at backend/onyx/db/models.py:1886-1890
// (refresh_freq, prune_freq) plus `ConnectorCredentialPair.auto_sync_options`
// at models.py:766-768.
//
// ARCHITECTURAL NOTE (see plan §1C): Onyx splits these across two
// tables (`Connector` for schedules, `ConnectorCredentialPair` for
// auto-sync options). Hiveloop already has `InConnection` as the
// identity row, so we park the schedule + per-connection ingest config
// here, keyed 1:1 on `in_connection_id`.
//
// DEVIATION: we introduce `PermSyncFreqSeconds` and
// `ExternalGroupSyncFreqSeconds` so the perm-sync and external-group
// loops are independently schedulable, mirroring Onyx's separate
// Celery beat tasks (see
// backend/onyx/background/celery/tasks/{doc_permission_syncing,
// external_group_syncing}) but exposing the cadence as first-class
// per-connection config rather than global env vars.
type RAGConnectionConfig struct {
	// InConnectionID is the 1:1 PK-ish key. Using InConnectionID as PK
	// (not a synthetic ID) keeps the invariant "one config row per
	// connection" physical.
	InConnectionID uuid.UUID `gorm:"type:uuid;primaryKey"`

	// OrgID — per universal RAG constraint. Indexed for org-wide
	// admin dashboards that list all connection configs in one org.
	OrgID uuid.UUID `gorm:"type:uuid;not null;index"`

	// IngestConfig — mirrors Onyx `Connector.connector_specific_config`
	// at models.py:1872. Shape is connector-dependent (e.g. repo list
	// for GitHub, workspace ID for Notion). Stored as JSONB so we can
	// index into it in SQL if future admin UIs need filtered views.
	IngestConfig model.JSON `gorm:"type:jsonb;not null;default:'{}'"`

	// RefreshFreqSeconds — port of Onyx `Connector.refresh_freq` at
	// models.py:1889. Null = run on demand only (no periodic refresh);
	// positive integer = seconds between refreshes. Validation via
	// ValidateRefreshFreq mirrors Onyx models.py:1919-1921 (≥ 60).
	RefreshFreqSeconds *int `gorm:"type:integer"`

	// PruneFreqSeconds — port of Onyx `Connector.prune_freq` at
	// models.py:1890. Validation via ValidatePruneFreq mirrors
	// Onyx models.py:1923-1928 (≥ 300).
	PruneFreqSeconds *int `gorm:"type:integer"`

	// PermSyncFreqSeconds — Hiveloop addition (see "DEVIATION" above).
	// Drives the doc-permission-sync loop. Null = default cadence.
	PermSyncFreqSeconds *int `gorm:"type:integer"`

	// ExternalGroupSyncFreqSeconds — Hiveloop addition. Drives the
	// external-group-sync loop. Null = default cadence.
	ExternalGroupSyncFreqSeconds *int `gorm:"type:integer"`

	// IndexingStart — port of Onyx `Connector.indexing_start`
	// (Onyx allows backfilling from a point in time). Null = index
	// everything the source returns.
	IndexingStart *time.Time `gorm:"type:timestamptz"`

	// KGProcessingEnabled — reserved for future KG port (Onyx
	// `Connector.kg_processing_enabled` at models.py:1880-1885).
	// Default false; we persist the column so migrations don't have
	// to touch this table once we implement KG.
	KGProcessingEnabled bool `gorm:"not null;default:false"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName — Hiveloop `rag_*` convention.
func (RAGConnectionConfig) TableName() string { return "rag_connection_configs" }

// ValidateRefreshFreq — port of Onyx `validate_refresh_freq` at
// backend/onyx/db/models.py:1916-1921. Returns a typed sentinel so
// callers can surface a 400 without string matching.
func (c *RAGConnectionConfig) ValidateRefreshFreq() error {
	if c.RefreshFreqSeconds == nil {
		return nil
	}
	if *c.RefreshFreqSeconds < 60 {
		return ErrRefreshFreqTooSmall
	}
	return nil
}

// ValidatePruneFreq — port of Onyx `validate_prune_freq` at
// backend/onyx/db/models.py:1923-1928.
func (c *RAGConnectionConfig) ValidatePruneFreq() error {
	if c.PruneFreqSeconds == nil {
		return nil
	}
	if *c.PruneFreqSeconds < 300 {
		return ErrPruneFreqTooSmall
	}
	return nil
}
