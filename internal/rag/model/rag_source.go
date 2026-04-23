package model

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

// Validation sentinel errors — direct port of the two ValueError branches at
// Onyx backend/onyx/db/models.py:1919-1929 (Connector.validate_refresh_freq
// and Connector.validate_prune_freq). Exported so API handlers can do typed
// errors.Is checks for request-validation responses.
var (
	ErrSourceRefreshFreqTooSmall = errors.New("refresh_freq must be greater than or equal to 1 minute (60 seconds)")
	ErrSourcePruneFreqTooSmall   = errors.New("prune_freq must be greater than or equal to 5 minutes (300 seconds)")
)

// RAGSourceKind identifies the kind of RAG source. It is a
// Hiveloop-only abstraction — Onyx's ConnectorCredentialPair at
// backend/onyx/db/models.py:723-837 only knows integration-backed
// sources. Hiveloop's superset adds WEBSITE (root URL + scrape
// config) and FILE_UPLOAD (direct upload) so every source kind is
// keyed off a single top-level row.
//
// Values are uppercase ASCII so Postgres dumps round-trip cleanly and tests
// can compare byte-identical strings across language boundaries.
type RAGSourceKind string

const (
	RAGSourceKindIntegration RAGSourceKind = "INTEGRATION"
	RAGSourceKindWebsite     RAGSourceKind = "WEBSITE"
	RAGSourceKindFileUpload  RAGSourceKind = "FILE_UPLOAD"
)

// IsValid returns true when k is one of the recognised RAGSourceKind
// values. Used by API input validation before a source is inserted.
func (k RAGSourceKind) IsValid() bool {
	switch k {
	case RAGSourceKindIntegration,
		RAGSourceKindWebsite,
		RAGSourceKindFileUpload:
		return true
	default:
		return false
	}
}

// String — mirrors the enum literal for logging / formatting.
func (k RAGSourceKind) String() string { return string(k) }

// RAGSourceStatus is the lifecycle state of a RAGSource. The
// scheduler scans rows with status IN ('ACTIVE','INITIAL_INDEXING')
// to pick work; everything else is either paused by the admin,
// never-connected, or on its way out.
//
// Port of Onyx ConnectorCredentialPairStatus at backend/onyx/db/enums.py:180-205,
// expanded with DISCONNECTED (nothing in Onyx that maps; represents "created
// but never successfully indexed") and ERROR (repeated failures visible to
// admin). Onyx uses in_repeated_error_state + INVALID for those shapes;
// we split them into a dedicated status + a boolean flag because the UI
// renders the two distinctly.
type RAGSourceStatus string

const (
	RAGSourceStatusDisconnected     RAGSourceStatus = "DISCONNECTED"
	RAGSourceStatusInitialIndexing  RAGSourceStatus = "INITIAL_INDEXING"
	RAGSourceStatusActive           RAGSourceStatus = "ACTIVE"
	RAGSourceStatusPaused           RAGSourceStatus = "PAUSED"
	RAGSourceStatusError            RAGSourceStatus = "ERROR"
	RAGSourceStatusDeleting         RAGSourceStatus = "DELETING"
)

// IsValid returns true when s is one of the recognised RAGSourceStatus
// values.
func (s RAGSourceStatus) IsValid() bool {
	switch s {
	case RAGSourceStatusDisconnected,
		RAGSourceStatusInitialIndexing,
		RAGSourceStatusActive,
		RAGSourceStatusPaused,
		RAGSourceStatusError,
		RAGSourceStatusDeleting:
		return true
	default:
		return false
	}
}

// IsActive returns true for statuses the scheduler treats as "should
// consider scheduling work against". Only ACTIVE and INITIAL_INDEXING
// qualify — a PAUSED source is explicitly parked; DISCONNECTED, ERROR,
// and DELETING all shouldn't receive new ingest tasks.
//
// Mirror of Onyx ConnectorCredentialPairStatus.is_active at
// backend/onyx/db/enums.py:203-204, narrowed to the two loop-drivers
// Hiveloop actually cares about.
func (s RAGSourceStatus) IsActive() bool {
	return s == RAGSourceStatusActive || s == RAGSourceStatusInitialIndexing
}

// String — mirror for logging / formatting.
func (s RAGSourceStatus) String() string { return string(s) }

// RAGSource is the top-level row for an organisation's RAG data
// source. A RAGSource has a Kind that dispatches to either an
// InConnection (integration-backed sources: GitHub, Notion, …) or a
// kind-specific config blob (WEBSITE, FILE_UPLOAD). Every RAG table
// keys on rag_source_id.
//
// Onyx analog: ConnectorCredentialPair at
// backend/onyx/db/models.py:723-837 (conceptual; ours is a superset
// that supports non-integration kinds). The connector-specific config
// JSON blob mirrors Connector.connector_specific_config at
// backend/onyx/db/models.py:1872.
//
// Interface clash note:
//
//	The Source interface (internal/rag/connectors/interfaces/connector.go)
//	defines four methods — SourceID(), OrgID(), SourceKind(), Config() —
//	returning string / string / string / json.RawMessage. This struct
//	cannot have exported fields with those names because Go disallows
//	method + field name collisions on the same receiver. The underlying
//	fields are therefore renamed (OrgID → OrgIDValue, Kind → KindValue,
//	Config → ConfigValue) and accessors matching the interface are
//	defined below. Column names are preserved via explicit
//	`gorm:"column:..."` tags.
type RAGSource struct {
	// ID — uuid PK, gen_random_uuid default per Hiveloop convention.
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	// OrgIDValue is the owning organization's UUID. Renamed from
	// OrgID to avoid clashing with the OrgID() accessor required by
	// the Source interface; the Postgres column name stays org_id.
	OrgIDValue uuid.UUID `gorm:"column:org_id;type:uuid;not null"`

	// KindValue is the RAGSourceKind discriminator. Renamed from Kind
	// for the same reason — the Source interface requires SourceKind()
	// as an accessor. Column name stays kind.
	KindValue RAGSourceKind `gorm:"column:kind;type:varchar(32);not null"`

	// Name — admin-facing label, e.g. "Engineering GitHub".
	Name string `gorm:"type:text;not null"`

	// Status — lifecycle state. See RAGSourceStatus.IsActive for the
	// scheduler gate predicate.
	Status RAGSourceStatus `gorm:"type:varchar(32);not null"`

	// Enabled — admin toggle. Scheduler skips sources with enabled=false
	// regardless of Status.
	Enabled bool `gorm:"not null;default:true"`

	// ConfigValue is the kind-specific JSON config blob. Renamed from
	// Config to avoid clashing with the Config() accessor required by
	// the Source interface. Column name stays config.
	//
	// Shape by kind:
	//   INTEGRATION : {} (connector-specific config lives on
	//                 InConnection's NangoConnectionID + the connector's
	//                 in-code defaults; future: repo allowlist for GitHub).
	//   WEBSITE     : {root_url, scrape_depth, include_patterns, exclude_patterns}
	//   FILE_UPLOAD : {} (TBD)
	ConfigValue model.JSON `gorm:"column:config;type:jsonb;not null;default:'{}'"`

	// InConnectionID — FK to in_connections.id. Non-null iff
	// Kind=INTEGRATION (CHECK-enforced). A unique partial index
	// guarantees one RAGSource per InConnection.
	InConnectionID *uuid.UUID `gorm:"type:uuid"`

	// AccessType — describes whether the indexed documents are PUBLIC
	// (visible to anyone in the org), PRIVATE (ACL gated), or SYNC
	// (ACLs kept fresh via perm_sync).
	AccessType AccessType `gorm:"type:varchar(16);not null"`

	// LastSuccessfulIndexTime — port of Onyx models.py:776-778. Finish
	// time (not start time) of the most recent successful ingest attempt.
	LastSuccessfulIndexTime *time.Time `gorm:"type:timestamptz"`

	// LastTimePermSync — port of Onyx models.py:769-771.
	LastTimePermSync *time.Time `gorm:"type:timestamptz"`

	// LastPruned — port of Onyx models.py:781-783.
	LastPruned *time.Time `gorm:"type:timestamptz"`

	// RefreshFreqSeconds — port of Onyx Connector.refresh_freq at
	// models.py:1889. Null = run on demand only.
	RefreshFreqSeconds *int `gorm:"type:integer"`

	// PruneFreqSeconds — port of Onyx Connector.prune_freq at
	// models.py:1890.
	PruneFreqSeconds *int `gorm:"type:integer"`

	// PermSyncFreqSeconds — Hiveloop addition. Default 6h (21600s),
	// matches Onyx's default perm-sync beat schedule.
	PermSyncFreqSeconds *int `gorm:"type:integer"`

	// TotalDocsIndexed — port of Onyx models.py:790.
	TotalDocsIndexed int `gorm:"not null;default:0"`

	// InRepeatedErrorState — port of Onyx CCPair's in_repeated_error_state
	// at models.py:744. Orthogonal to Status; an ACTIVE source can still
	// be flagged as repeatedly failing so the admin UI surfaces it.
	InRepeatedErrorState bool `gorm:"not null;default:false"`

	// DeletionFailureMessage — port of Onyx models.py:749.
	DeletionFailureMessage *string `gorm:"type:text"`

	// CreatorID — port of Onyx models.py:827. Nullable: deleting the
	// user must not cascade into tombstoning the source, which is
	// owned by the org.
	CreatorID *uuid.UUID `gorm:"type:uuid"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName pins the Postgres table name.
func (RAGSource) TableName() string { return "rag_sources" }

// SourceID satisfies interfaces.Source. Returns the UUID rendered as a
// string; the connector layer uses this as a lock key and attempt-
// attribution identifier (no UUID type dependency leaks across package
// boundaries).
func (s *RAGSource) SourceID() string { return s.ID.String() }

// OrgID satisfies interfaces.Source.
func (s *RAGSource) OrgID() string { return s.OrgIDValue.String() }

// SourceKind satisfies interfaces.Source. Returns the connector kind
// as a string (matches Connector.Kind() and the InIntegration.Provider
// values seeded at migrate time).
func (s *RAGSource) SourceKind() string { return string(s.KindValue) }

// Config satisfies interfaces.Source. Marshals the internal JSONB map into
// json.RawMessage so the connector layer never has to depend on internal/model.
// Returns an empty object (`{}`) when the map is nil or empty — mirroring
// the not-null-default-'{}' column behavior.
func (s *RAGSource) Config() json.RawMessage {
	if s.ConfigValue == nil || len(s.ConfigValue) == 0 {
		return json.RawMessage(`{}`)
	}
	raw, err := json.Marshal(s.ConfigValue)
	if err != nil {
		// model.JSON is a map[string]any that has always serialised
		// cleanly in every code path we drive it through; a non-nil
		// error here would indicate a programming error (e.g. putting
		// a channel in the map) that we surface as invalid JSON.
		return json.RawMessage(`{}`)
	}
	return raw
}

// ValidateRefreshFreq — port of Onyx Connector.validate_refresh_freq at
// backend/onyx/db/models.py:1919-1921. Returns a typed sentinel so
// callers can surface a 422 without string matching.
func (s *RAGSource) ValidateRefreshFreq() error {
	if s.RefreshFreqSeconds == nil {
		return nil
	}
	if *s.RefreshFreqSeconds < 60 {
		return ErrSourceRefreshFreqTooSmall
	}
	return nil
}

// ValidatePruneFreq — port of Onyx Connector.validate_prune_freq at
// backend/onyx/db/models.py:1923-1928.
func (s *RAGSource) ValidatePruneFreq() error {
	if s.PruneFreqSeconds == nil {
		return nil
	}
	if *s.PruneFreqSeconds < 300 {
		return ErrSourcePruneFreqTooSmall
	}
	return nil
}
