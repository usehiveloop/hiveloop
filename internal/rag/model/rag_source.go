package model

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/usehiveloop/hiveloop/internal/model"
)

var (
	ErrSourceRefreshFreqTooSmall = errors.New("refresh_freq must be greater than or equal to 1 minute (60 seconds)")
	ErrSourcePruneFreqTooSmall   = errors.New("prune_freq must be greater than or equal to 5 minutes (300 seconds)")
)

type RAGSourceKind string

const (
	RAGSourceKindIntegration RAGSourceKind = "INTEGRATION"
	RAGSourceKindWebsite     RAGSourceKind = "WEBSITE"
	RAGSourceKindFileUpload  RAGSourceKind = "FILE_UPLOAD"
)

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

func (k RAGSourceKind) String() string { return string(k) }

type RAGSourceStatus string

const (
	RAGSourceStatusDisconnected    RAGSourceStatus = "DISCONNECTED"
	RAGSourceStatusInitialIndexing RAGSourceStatus = "INITIAL_INDEXING"
	RAGSourceStatusActive          RAGSourceStatus = "ACTIVE"
	RAGSourceStatusPaused          RAGSourceStatus = "PAUSED"
	RAGSourceStatusError           RAGSourceStatus = "ERROR"
	RAGSourceStatusDeleting        RAGSourceStatus = "DELETING"
)

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

// IsActive: only ACTIVE and INITIAL_INDEXING are scheduler-eligible —
// PAUSED is admin-parked, DISCONNECTED/ERROR/DELETING shouldn't
// receive new ingest tasks.
func (s RAGSourceStatus) IsActive() bool {
	return s == RAGSourceStatusActive || s == RAGSourceStatusInitialIndexing
}

func (s RAGSourceStatus) String() string { return string(s) }

// RAGSource fields OrgIDValue / KindValue / ConfigValue are renamed
// from OrgID / Kind / Config because Go disallows method+field name
// collisions on the same receiver, and the Source interface defines
// methods of those names. Column names are preserved via
// `gorm:"column:..."` tags.
type RAGSource struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	OrgIDValue uuid.UUID `gorm:"column:org_id;type:uuid;not null"`

	KindValue RAGSourceKind `gorm:"column:kind;type:varchar(32);not null"`

	Name string `gorm:"type:text;not null"`

	Status RAGSourceStatus `gorm:"type:varchar(32);not null"`

	Enabled bool `gorm:"not null;default:true"`

	ConfigValue model.JSON `gorm:"column:config;type:jsonb;not null;default:'{}'"`

	// InConnectionID is non-null iff Kind=INTEGRATION (CHECK-enforced).
	InConnectionID *uuid.UUID `gorm:"type:uuid"`

	AccessType AccessType `gorm:"type:varchar(16);not null"`

	// IndexingStart caps the initial-index window for pathological
	// sources (large mirrors). NULL = no floor.
	IndexingStart *time.Time `gorm:"type:timestamptz"`

	// LastSuccessfulIndexTime is the finish time (not start time) of
	// the most recent successful attempt.
	LastSuccessfulIndexTime *time.Time `gorm:"type:timestamptz"`

	LastTimePermSync *time.Time `gorm:"type:timestamptz"`

	LastPruned *time.Time `gorm:"type:timestamptz"`

	// RefreshFreqSeconds: null = run on demand only.
	RefreshFreqSeconds *int `gorm:"type:integer"`

	PruneFreqSeconds *int `gorm:"type:integer"`

	PermSyncFreqSeconds *int `gorm:"type:integer"`

	TotalDocsIndexed int `gorm:"not null;default:0"`

	// InRepeatedErrorState is orthogonal to Status — an ACTIVE source
	// can still be flagged as repeatedly failing.
	InRepeatedErrorState bool `gorm:"not null;default:false"`

	DeletionFailureMessage *string `gorm:"type:text"`

	// CreatorID is nullable so deleting the user does not cascade into
	// tombstoning the org-owned source.
	CreatorID *uuid.UUID `gorm:"type:uuid"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (RAGSource) TableName() string { return "rag_sources" }

func (s *RAGSource) SourceID() string { return s.ID.String() }

func (s *RAGSource) OrgID() string { return s.OrgIDValue.String() }

func (s *RAGSource) SourceKind() string { return string(s.KindValue) }

// Config returns `{}` when the map is nil/empty so the column's
// not-null-default-'{}' behavior is preserved at the interface boundary.
func (s *RAGSource) Config() json.RawMessage {
	if s.ConfigValue == nil || len(s.ConfigValue) == 0 {
		return json.RawMessage(`{}`)
	}
	raw, err := json.Marshal(s.ConfigValue)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func (s *RAGSource) ValidateRefreshFreq() error {
	if s.RefreshFreqSeconds == nil {
		return nil
	}
	if *s.RefreshFreqSeconds < 60 {
		return ErrSourceRefreshFreqTooSmall
	}
	return nil
}

func (s *RAGSource) ValidatePruneFreq() error {
	if s.PruneFreqSeconds == nil {
		return nil
	}
	if *s.PruneFreqSeconds < 300 {
		return ErrSourcePruneFreqTooSmall
	}
	return nil
}
