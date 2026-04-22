package interfaces

// This file contains the neutral document shapes every connector produces.
// They travel across JSON boundaries (checkpoint storage, test fixtures,
// gRPC proto conversion) so their field order and JSON tags are load-bearing.

import "time"

// Document is the neutral shape every connector produces. The rag-engine
// gRPC IngestBatch request is built from these at the scheduler boundary
// (Tranche 3C). Kept in sync with proto/rag_engine.proto's DocumentToIngest
// message.
//
// Onyx analog: Document at
// backend/onyx/connectors/models.py:348-383 (extends DocumentBase at :185-279).
//
// We flatten Onyx's multi-section-subtype hierarchy (TextSection / ImageSection
// / TabularSection) into a single Section type in this tranche — only text
// is indexed right now, and tabular/image sections can be added later without
// breaking the contract.
type Document struct {
	// DocID is the source-given opaque identifier. Matches RAGDocument.ID
	// (Phase 1A) and the upstream Onyx Document.id field.
	DocID string `json:"doc_id"`

	// SemanticID is the human-readable display label (Onyx
	// semantic_identifier at models.py:191).
	SemanticID string `json:"semantic_id"`

	// Link points back to the source-of-truth view of this document (e.g.
	// the GitHub PR URL). Optional — empty string means "no external link."
	Link string `json:"link"`

	// Sections is the ordered list of content blocks making up the
	// document. Each section becomes one or more chunks at indexing time.
	// Empty sections are permitted but skipped by the chunker per the
	// Phase 2E contract.
	Sections []Section `json:"sections"`

	// ACL is a list of opaque pre-prefixed tokens as produced by
	// onyx/access/utils.py and its Hiveloop port at internal/rag/acl.
	// The scheduler feeds these straight through to Rust-side ACL
	// matching — this layer does NOT prefix or transform them.
	ACL []string `json:"acl,omitempty"`

	// IsPublic, when true, marks the document as visible to all users in
	// the org regardless of the ACL list.
	IsPublic bool `json:"is_public"`

	// DocUpdatedAt is the source-reported last-modified timestamp. Used by
	// the poll-window logic in CheckpointedConnector implementations.
	DocUpdatedAt *time.Time `json:"doc_updated_at,omitempty"`

	// Metadata is a flat string->string map of source-specific fields
	// surfaced to search + chat layers. Onyx supports string|list[str]
	// values (DocumentBase.metadata at models.py:194); we intentionally
	// restrict to string-only here and leave list splitting to the
	// caller.
	Metadata map[string]string `json:"metadata,omitempty"`

	// PrimaryOwners is the list of primary-owner email addresses
	// (authors, creators). Port of DocumentBase.primary_owners at
	// models.py:209. Strings rather than structs: we don't carry the
	// full BasicExpertInfo shape — just the email, which is what ACL
	// + display layers actually need.
	PrimaryOwners []string `json:"primary_owners,omitempty"`

	// SecondaryOwners is the list of secondary-owner email addresses
	// (assignees, reviewers). Port of DocumentBase.secondary_owners at
	// models.py:211.
	SecondaryOwners []string `json:"secondary_owners,omitempty"`
}

// Section is one content block within a Document. One Document has one or
// more Sections; the chunker (Phase 2E) splits long Sections and combines
// short ones into chunks.
//
// Onyx analog: TextSection at backend/onyx/connectors/models.py:56-63 —
// we port only the text-bearing shape; image and tabular sections are
// deferred to future tranches.
//
// Contract: Text may be empty. The chunker MUST skip empty sections without
// raising. This is pinned by TestSectionEmpty_AllowedByContract.
type Section struct {
	// Text is the indexable content of this section. May be empty.
	Text string `json:"text"`

	// Link is an optional per-section deep-link (e.g. GitHub comment URL).
	// Falls back to the parent Document.Link at render time.
	Link string `json:"link,omitempty"`

	// Title is an optional heading for this section (Onyx
	// Section.heading at models.py:53).
	Title string `json:"title,omitempty"`
}

// SlimDocument is the minimal shape produced by SlimConnector.ListAllSlim.
// Used by the pruning loop (Tranche 3C) to diff against the indexed set
// and detect source-side deletions. ExternalAccess is optional — set when
// the slim listing can cheaply surface permission info (so a separate
// perm_sync pass is unnecessary for that document).
//
// Onyx analog: SlimDocument at backend/onyx/connectors/models.py:410-413.
type SlimDocument struct {
	// DocID matches Document.DocID one-to-one.
	DocID string `json:"doc_id"`

	// ExternalAccess, if non-nil, is the current ACL for this document
	// as observed during the slim listing. The scheduler may use this to
	// short-circuit a perm_sync pass.
	ExternalAccess *ExternalAccess `json:"external_access,omitempty"`
}
