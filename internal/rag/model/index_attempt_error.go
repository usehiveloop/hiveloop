package model

import (
	"time"

	"github.com/google/uuid"
)

// RAGIndexAttemptError records a single per-document (or per-entity,
// per-time-range) failure that happened inside a RAGIndexAttempt.
// Multiple errors per attempt are expected — one attempt may partially
// succeed (Status=completed_with_errors) and have a fleet of these rows
// explaining which documents the admin needs to retry.
//
// Verbatim port of Onyx `IndexAttemptError` at
// backend/onyx/db/models.py:2399-2438.
//
// DEVIATIONS vs Onyx:
//   - PK is uuid, not autoincrement int (Hiveloop convention).
//   - `connector_credential_pair_id` becomes `in_connection_id`.
//   - CASCADE on the attempt FK is explicit — if the parent attempt
//     goes away (e.g. org deletion cascades through it), the error log
//     goes with it. Onyx has the same effective behavior via SQLAlchemy
//     `cascade="all, delete-orphan"` at models.py:2292.
type RAGIndexAttemptError struct {
	// ID — Onyx models.py:2402.
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	// OrgID — Hiveloop addition for row-level tenancy + fast
	// org-scoped error listings in the admin UI.
	OrgID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`

	// IndexAttemptID — Onyx models.py:2404-2408. CASCADE so the error
	// lifecycle tracks the parent attempt (Onyx achieves the same via
	// SQLAlchemy cascade="all, delete-orphan" at models.py:2290-2294).
	IndexAttemptID uuid.UUID    `gorm:"type:uuid;not null;index"`
	IndexAttempt   RAGIndexAttempt `gorm:"foreignKey:IndexAttemptID;constraint:OnDelete:CASCADE"`

	// RAGSourceID — adapts Onyx `connector_credential_pair_id`
	// (models.py:2409-2413). Mirror of what's on the parent attempt,
	// denormalised so the admin UI can filter errors by source without
	// a join.
	//
	// Phase 3A swap (was InConnectionID).
	RAGSourceID uuid.UUID `gorm:"type:uuid;not null;index"`

	// DocumentID / DocumentLink — Onyx models.py:2415-2416. Either/or
	// both may be null — an error raised before we had a document
	// identity (e.g. upstream 500 on the list endpoint) won't have
	// either.
	DocumentID   *string `gorm:"type:text"`
	DocumentLink *string `gorm:"type:text"`

	// EntityID / FailedTimeRangeStart / FailedTimeRangeEnd —
	// Onyx models.py:2418-2424. For time-range connectors (Slack,
	// calendar) this is how we identify "the slice that failed" so a
	// subsequent retry can target just that window.
	EntityID             *string    `gorm:"type:text"`
	FailedTimeRangeStart *time.Time
	FailedTimeRangeEnd   *time.Time

	// FailureMessage — the human-readable error message. Non-null per
	// Onyx (an error row without a message is useless).
	// Onyx models.py:2426.
	FailureMessage string `gorm:"type:text;not null"`

	// IsResolved — admin can mark an error as acknowledged/handled
	// without deleting the record. Onyx models.py:2427.
	IsResolved bool `gorm:"not null;default:false"`

	// ErrorType — optional classifier (e.g. "PermissionDenied",
	// "RateLimited"). Onyx models.py:2429.
	ErrorType *string `gorm:"type:text"`

	// TimeCreated — server-defaulted so inserting apps never have to
	// supply it. Onyx models.py:2431-2434.
	TimeCreated time.Time `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

// TableName pins the Postgres table name.
func (RAGIndexAttemptError) TableName() string { return "rag_index_attempt_errors" }
