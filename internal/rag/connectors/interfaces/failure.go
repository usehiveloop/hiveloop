package interfaces

import "time"

// DocumentFailure identifies a specific document that failed during a
// connector run. Exactly one of {DocumentFailure, EntityFailure} is set
// per ConnectorFailure — see the ConnectorFailure godoc for the
// invariant.
//
// Onyx analog: DocumentFailure at
// backend/onyx/connectors/models.py:476-478.
type DocumentFailure struct {
	// DocID matches Document.DocID — identifies which document would
	// have been produced had the fetch succeeded.
	DocID string `json:"doc_id"`

	// DocumentLink is the source-side URL for display in admin UI.
	// Optional — empty string is permitted for sources that don't
	// expose URLs.
	DocumentLink string `json:"document_link,omitempty"`
}

// EntityFailure identifies a non-document entity (e.g. a repo, a page
// cursor, a time range) whose retrieval failed, without pinning the
// failure to a single document.
//
// Onyx analog: EntityFailure at
// backend/onyx/connectors/models.py:481-483.
type EntityFailure struct {
	// EntityID is a connector-defined opaque identifier (e.g. a repo
	// name, a Confluence space key). No format is enforced here.
	EntityID string `json:"entity_id"`

	// MissedTimeRangeStart / MissedTimeRangeEnd, when both non-nil,
	// describe a poll window that failed and must be retried. The
	// scheduler's retry loop uses these to build re-poll tasks.
	MissedTimeRangeStart *time.Time `json:"missed_time_range_start,omitempty"`
	MissedTimeRangeEnd   *time.Time `json:"missed_time_range_end,omitempty"`
}

// ConnectorFailure wraps a failure that should be logged but not abort
// the overall connector run. The scheduler drains these into
// RAGIndexAttemptError rows (Phase 1B).
//
// Invariant: exactly one of {FailedDocument, FailedEntity} is non-nil.
// This mirrors Onyx's model_validator at
// backend/onyx/connectors/models.py:494-504. Go doesn't have
// Pydantic-style constructor-time validation, so enforcement lives in
// the constructor helpers below.
//
// Cause, if non-nil, carries the underlying error so callers can use
// errors.Is / errors.As to unwrap. It is deliberately NOT marshaled in
// JSON — the string FailureMessage is the durable representation that
// survives the Postgres round-trip.
type ConnectorFailure struct {
	// FailedDocument is set when the failure is pinned to one doc.
	FailedDocument *DocumentFailure `json:"failed_document,omitempty"`

	// FailedEntity is set when the failure is a non-document entity.
	FailedEntity *EntityFailure `json:"failed_entity,omitempty"`

	// FailureMessage is the durable human-readable description.
	// Always populated.
	FailureMessage string `json:"failure_message"`

	// Cause is the underlying Go error, if any. Never marshaled.
	// Kept for errors.Is/errors.As in downstream handlers.
	Cause error `json:"-"`
}

// NewDocumentFailure constructs a ConnectorFailure pinned to a single
// document. Use when the fetch of a specific doc failed — e.g. GitHub
// returned 403 on one PR while the rest of the repo succeeded.
func NewDocumentFailure(docID, docLink, msg string, cause error) *ConnectorFailure {
	return &ConnectorFailure{
		FailedDocument: &DocumentFailure{DocID: docID, DocumentLink: docLink},
		FailureMessage: msg,
		Cause:          cause,
	}
}

// NewEntityFailure constructs a ConnectorFailure for a non-document
// entity (repo, cursor, time range). Use when the failure can't be
// attributed to one document.
func NewEntityFailure(entityID, msg string, missedStart, missedEnd *time.Time, cause error) *ConnectorFailure {
	return &ConnectorFailure{
		FailedEntity: &EntityFailure{
			EntityID:             entityID,
			MissedTimeRangeStart: missedStart,
			MissedTimeRangeEnd:   missedEnd,
		},
		FailureMessage: msg,
		Cause:          cause,
	}
}

// DocumentOrFailure is a discriminated union: exactly one of
// {Doc, Failure} is non-nil per value. Emitted on the channel returned
// by CheckpointedConnector.LoadFromCheckpoint so the scheduler can
// interleave successful fetches with per-doc failures without aborting
// the run.
//
// Use NewDocResult / NewDocFailure to construct — they enforce the
// mutual-exclusion invariant that tests (and runtime sanity) rely on.
type DocumentOrFailure struct {
	Doc     *Document         `json:"doc,omitempty"`
	Failure *ConnectorFailure `json:"failure,omitempty"`
}

// NewDocResult wraps a successful Document fetch. Panics on nil to
// catch programming errors at the call site rather than at the
// consumer.
func NewDocResult(d *Document) DocumentOrFailure {
	if d == nil {
		panic("interfaces: NewDocResult called with nil *Document")
	}
	return DocumentOrFailure{Doc: d}
}

// NewDocFailure wraps a per-doc failure. Panics on nil.
func NewDocFailure(f *ConnectorFailure) DocumentOrFailure {
	if f == nil {
		panic("interfaces: NewDocFailure called with nil *ConnectorFailure")
	}
	return DocumentOrFailure{Failure: f}
}

// SlimDocOrFailure is the union variant for SlimConnector.ListAllSlim.
// Same shape/invariants as DocumentOrFailure, but wrapping
// SlimDocument (used by the prune loop — see Tranche 3C).
type SlimDocOrFailure struct {
	Slim    *SlimDocument     `json:"slim,omitempty"`
	Failure *ConnectorFailure `json:"failure,omitempty"`
}

// NewSlimResult wraps a successful SlimDocument. Panics on nil.
func NewSlimResult(s *SlimDocument) SlimDocOrFailure {
	if s == nil {
		panic("interfaces: NewSlimResult called with nil *SlimDocument")
	}
	return SlimDocOrFailure{Slim: s}
}

// NewSlimFailure wraps a per-slim-doc failure. Panics on nil.
func NewSlimFailure(f *ConnectorFailure) SlimDocOrFailure {
	if f == nil {
		panic("interfaces: NewSlimFailure called with nil *ConnectorFailure")
	}
	return SlimDocOrFailure{Failure: f}
}

// DocExternalAccessOrFailure is the union variant for
// PermSyncConnector.SyncDocPermissions.
type DocExternalAccessOrFailure struct {
	Access  *DocExternalAccess `json:"access,omitempty"`
	Failure *ConnectorFailure  `json:"failure,omitempty"`
}

// NewAccessResult wraps a successful DocExternalAccess. Panics on nil.
func NewAccessResult(a *DocExternalAccess) DocExternalAccessOrFailure {
	if a == nil {
		panic("interfaces: NewAccessResult called with nil *DocExternalAccess")
	}
	return DocExternalAccessOrFailure{Access: a}
}

// NewAccessFailure wraps a failure during doc-perm sync. Panics on nil.
func NewAccessFailure(f *ConnectorFailure) DocExternalAccessOrFailure {
	if f == nil {
		panic("interfaces: NewAccessFailure called with nil *ConnectorFailure")
	}
	return DocExternalAccessOrFailure{Failure: f}
}

// ExternalGroupOrFailure is the union variant for
// PermSyncConnector.SyncExternalGroups.
type ExternalGroupOrFailure struct {
	Group   *ExternalGroup    `json:"group,omitempty"`
	Failure *ConnectorFailure `json:"failure,omitempty"`
}

// NewGroupResult wraps a successful ExternalGroup. Panics on nil.
func NewGroupResult(g *ExternalGroup) ExternalGroupOrFailure {
	if g == nil {
		panic("interfaces: NewGroupResult called with nil *ExternalGroup")
	}
	return ExternalGroupOrFailure{Group: g}
}

// NewGroupFailure wraps a failure during group sync. Panics on nil.
func NewGroupFailure(f *ConnectorFailure) ExternalGroupOrFailure {
	if f == nil {
		panic("interfaces: NewGroupFailure called with nil *ConnectorFailure")
	}
	return ExternalGroupOrFailure{Failure: f}
}
